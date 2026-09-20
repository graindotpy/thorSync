package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
)

const (
	maxJWKSBytes = 1 << 20
	maxJWKSKeys  = 64
)

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func (a *Authenticator) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	a.keysMu.Lock()
	defer a.keysMu.Unlock()

	now := a.now()
	if now.Before(a.keysExpires) {
		if key := a.keys[keyID]; key != nil {
			return key, nil
		}
	}

	// An expired cache, an empty cache, or an unknown kid all trigger a fresh
	// fetch. The mutex deliberately serializes refreshes to avoid a thundering
	// herd during key rotation.
	keys, err := a.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	a.keys = keys
	a.keysExpires = now.Add(a.cacheTTL)
	key := keys[keyID]
	if key == nil {
		return nil, errors.New("unknown key ID")
	}
	return key, nil
}

func (a *Authenticator) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.jwksURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxJWKSBytes {
		return nil, errors.New("JWKS response too large")
	}
	var document jwksDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, errors.New("invalid JWKS JSON")
	}
	if len(document.Keys) == 0 || len(document.Keys) > maxJWKSKeys {
		return nil, errors.New("invalid JWKS key count")
	}

	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, encoded := range document.Keys {
		if encoded.KeyType != "RSA" || encoded.KeyID == "" {
			continue
		}
		if encoded.Algorithm != "" && encoded.Algorithm != "RS256" {
			continue
		}
		if encoded.Use != "" && encoded.Use != "sig" {
			continue
		}
		key, err := rsaKey(encoded)
		if err != nil {
			continue
		}
		if _, duplicate := keys[encoded.KeyID]; duplicate {
			return nil, errors.New("duplicate JWKS key ID")
		}
		keys[encoded.KeyID] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS contains no usable RSA signing keys")
	}
	return keys, nil
}

func rsaKey(encoded jwk) (*rsa.PublicKey, error) {
	modulusBytes, err := base64.RawURLEncoding.DecodeString(encoded.Modulus)
	if err != nil || len(modulusBytes) == 0 {
		return nil, errors.New("invalid RSA modulus")
	}
	modulus := new(big.Int).SetBytes(modulusBytes)
	if modulus.Sign() <= 0 || modulus.BitLen() < 2048 {
		return nil, errors.New("RSA modulus is too small")
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(encoded.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 8 {
		return nil, errors.New("invalid RSA exponent")
	}
	var exponent uint64
	for _, part := range exponentBytes {
		exponent = exponent<<8 | uint64(part)
	}
	if exponent < 3 || exponent%2 == 0 || exponent > uint64(^uint(0)>>1) {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: modulus, E: int(exponent)}, nil
}
