package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testIssuer   = "https://team.example.cloudflareaccess.com"
	testAudience = "9e1371a0d6c84dc89ba5f1f6d3fcb3b4"
	testEmail    = "admin@example.com"
)

var testNow = time.Unix(2_000_000_000, 0).UTC()

type keyServer struct {
	server *httptest.Server
	count  atomic.Int64
	mu     sync.RWMutex
	keys   []jwk
}

func newKeyServer(t *testing.T, keys ...jwk) *keyServer {
	t.Helper()
	fixture := &keyServer{keys: keys}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fixture.count.Add(1)
		fixture.mu.RLock()
		defer fixture.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwksDocument{Keys: fixture.keys}); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (s *keyServer) setKeys(keys ...jwk) {
	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
}

func generateKey(t *testing.T, keyID string) (*rsa.PrivateKey, jwk) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	return privateKey, jwk{
		KeyType:   "RSA",
		KeyID:     keyID,
		Use:       "sig",
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func newTestAuthenticator(t *testing.T, server *keyServer, health ...string) *Authenticator {
	t.Helper()
	authenticator, err := New(Config{
		Issuer:      testIssuer,
		Audience:    testAudience,
		AdminEmail:  "ADMIN@example.com",
		JWKSURL:     server.server.URL,
		HealthPaths: health,
		Clock:       func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatalf("create authenticator: %v", err)
	}
	return authenticator
}

func validClaims() map[string]any {
	return map[string]any{
		"iss":   testIssuer,
		"aud":   []string{"another-audience", testAudience},
		"email": testEmail,
		"sub":   "user-123",
		"type":  "app",
		"exp":   testNow.Add(5 * time.Minute).Unix(),
		"nbf":   testNow.Add(-time.Minute).Unix(),
	}
}

func signedToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	headerBytes, err := json.Marshal(map[string]any{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	digest := sha256.Sum256([]byte(header + "." + payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestValidateAcceptsValidApplicationToken(t *testing.T) {
	key, publicJWK := generateKey(t, "active")
	server := newKeyServer(t, publicJWK)
	authenticator := newTestAuthenticator(t, server)

	principal, err := authenticator.Validate(context.Background(), signedToken(t, key, "active", validClaims()))
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if principal.Email != testEmail || principal.Subject != "user-123" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if !principal.ExpiresAt.Equal(testNow.Add(5 * time.Minute)) {
		t.Fatalf("ExpiresAt = %v", principal.ExpiresAt)
	}
	if got := server.count.Load(); got != 1 {
		t.Fatalf("JWKS requests = %d, want 1", got)
	}

	// A known key is served from the fresh cache.
	if _, err := authenticator.Validate(context.Background(), signedToken(t, key, "active", validClaims())); err != nil {
		t.Fatalf("second Validate() error = %v", err)
	}
	if got := server.count.Load(); got != 1 {
		t.Fatalf("JWKS requests after cache hit = %d, want 1", got)
	}
}

func TestValidateRejectsInvalidIdentityAndTimeClaims(t *testing.T) {
	key, publicJWK := generateKey(t, "active")
	server := newKeyServer(t, publicJWK)
	authenticator := newTestAuthenticator(t, server)

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "wrong issuer",
			mutate: func(claims map[string]any) {
				claims["iss"] = "https://other.cloudflareaccess.com"
			},
		},
		{
			name: "wrong audience",
			mutate: func(claims map[string]any) {
				claims["aud"] = "some-other-application"
			},
		},
		{
			name: "wrong email",
			mutate: func(claims map[string]any) {
				claims["email"] = "other@example.com"
			},
		},
		{
			name: "noncanonical email",
			mutate: func(claims map[string]any) {
				claims["email"] = "ADMIN@example.com"
			},
		},
		{
			name: "expired",
			mutate: func(claims map[string]any) {
				claims["exp"] = testNow.Unix()
			},
		},
		{
			name: "not active",
			mutate: func(claims map[string]any) {
				claims["nbf"] = testNow.Add(time.Second).Unix()
			},
		},
		{
			name: "service token",
			mutate: func(claims map[string]any) {
				claims["type"] = "service-token"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validClaims()
			test.mutate(claims)
			if principal, err := authenticator.Validate(context.Background(), signedToken(t, key, "active", claims)); err == nil {
				t.Fatalf("Validate() accepted token as %#v", principal)
			}
		})
	}
}

func TestValidateRefreshesJWKSForUnknownKeyID(t *testing.T) {
	firstKey, firstJWK := generateKey(t, "first")
	secondKey, secondJWK := generateKey(t, "second")
	server := newKeyServer(t, firstJWK)
	authenticator := newTestAuthenticator(t, server)

	if _, err := authenticator.Validate(context.Background(), signedToken(t, firstKey, "first", validClaims())); err != nil {
		t.Fatalf("initial Validate() error = %v", err)
	}
	server.setKeys(secondJWK)
	if _, err := authenticator.Validate(context.Background(), signedToken(t, secondKey, "second", validClaims())); err != nil {
		t.Fatalf("Validate() after rotation error = %v", err)
	}
	if got := server.count.Load(); got != 2 {
		t.Fatalf("JWKS requests = %d, want 2", got)
	}

	if _, err := authenticator.Validate(context.Background(), signedToken(t, secondKey, "second", validClaims())); err != nil {
		t.Fatalf("cached rotated key error = %v", err)
	}
	if got := server.count.Load(); got != 2 {
		t.Fatalf("JWKS requests after rotated cache hit = %d, want 2", got)
	}
}

func TestMiddlewareAuthenticatesAndExemptsOnlyReadHealthPaths(t *testing.T) {
	key, publicJWK := generateKey(t, "active")
	server := newKeyServer(t, publicJWK)
	authenticator := newTestAuthenticator(t, server, "/health/live", "/health/ready")

	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/me" {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal.Email != testEmail {
				t.Errorf("missing principal: %#v, %v", principal, ok)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	healthRequest := httptest.NewRequest(http.MethodGet, "https://sync.example.com/health/live", nil)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusNoContent {
		t.Fatalf("health status = %d", healthResponse.Code)
	}

	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health/live/"},
		{http.MethodPost, "/health/live"},
		{http.MethodGet, "/api/me"},
	} {
		request := httptest.NewRequest(test.method, "https://sync.example.com"+test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want 401", test.method, test.path, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "https://sync.example.com/api/me", nil)
	request.Header.Set(defaultAssertionHeader, signedToken(t, key, "active", validClaims()))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d, body = %q", response.Code, response.Body.String())
	}
}

func TestMiddlewareFailsClosedWhenJWKSUnavailable(t *testing.T) {
	key, publicJWK := generateKey(t, "active")
	server := newKeyServer(t, publicJWK)
	authenticator := newTestAuthenticator(t, server)
	server.server.Close()

	handler := authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))
	request := httptest.NewRequest(http.MethodGet, "https://sync.example.com/api/games", nil)
	request.Header.Set(defaultAssertionHeader, signedToken(t, key, "active", validClaims()))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}
