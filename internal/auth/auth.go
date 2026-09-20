// Package auth validates Cloudflare Access application tokens and provides
// HTTP middleware for the ThorSync web application.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAssertionHeader = "Cf-Access-Jwt-Assertion"
	defaultCacheTTL        = time.Hour
	defaultHTTPTimeout     = 5 * time.Second
	maxTokenBytes          = 64 << 10
)

// Config contains the fixed values against which every Access token is
// checked. HealthPaths are exact URL paths which may be read without a token.
// Only GET and HEAD requests are exempted.
type Config struct {
	Issuer      string
	Audience    string
	AdminEmail  string
	JWKSURL     string
	HealthPaths []string

	// Header overrides the Cloudflare assertion header. It is primarily useful
	// when ThorSync is deployed behind a proxy which renames headers.
	Header string
	// CacheTTL controls how long a successful JWKS response remains fresh.
	CacheTTL time.Duration
	// HTTPClient is used to retrieve the JWKS. A client with a five second
	// timeout is used when this is nil.
	HTTPClient *http.Client
	// Clock is optional and is intended for deterministic tests.
	Clock func() time.Time
}

// Principal is the authenticated Cloudflare Access user attached to a request.
type Principal struct {
	Email     string
	Subject   string
	ExpiresAt time.Time
}

// Authenticator validates Cloudflare Access JWT assertions. It is safe for
// concurrent use.
type Authenticator struct {
	issuer     string
	audience   string
	adminEmail string
	jwksURL    *url.URL
	header     string
	health     map[string]struct{}
	cacheTTL   time.Duration
	client     *http.Client
	now        func() time.Time

	keysMu      sync.Mutex
	keys        map[string]*rsa.PublicKey
	keysExpires time.Time
}

// Health verifies that a usable JWKS is cached or can be fetched. It is used
// only for authenticated diagnostics; request validation remains fail closed.
func (a *Authenticator) Health(ctx context.Context) error {
	a.keysMu.Lock()
	defer a.keysMu.Unlock()
	if len(a.keys) > 0 && a.now().Before(a.keysExpires) {
		return nil
	}
	keys, err := a.fetchKeys(ctx)
	if err != nil {
		return err
	}
	a.keys = keys
	a.keysExpires = a.now().Add(a.cacheTTL)
	return nil
}

// New constructs an Authenticator. Key retrieval is deliberately lazy so a
// temporary JWKS outage does not prevent the HTTP server from starting; token
// validation still fails closed while the outage lasts.
func New(cfg Config) (*Authenticator, error) {
	issuer := strings.TrimSpace(cfg.Issuer)
	if issuer == "" {
		return nil, errors.New("auth: issuer is required")
	}
	audience := strings.TrimSpace(cfg.Audience)
	if audience == "" {
		return nil, errors.New("auth: audience is required")
	}
	adminEmail := strings.ToLower(strings.TrimSpace(cfg.AdminEmail))
	if adminEmail == "" {
		return nil, errors.New("auth: admin email is required")
	}
	if strings.ContainsAny(adminEmail, "\r\n") {
		return nil, errors.New("auth: invalid admin email")
	}

	jwksURL, err := url.Parse(strings.TrimSpace(cfg.JWKSURL))
	if err != nil || jwksURL.Scheme == "" || jwksURL.Host == "" {
		return nil, errors.New("auth: JWKS URL must be an absolute URL")
	}
	if jwksURL.Scheme != "https" && jwksURL.Scheme != "http" {
		return nil, errors.New("auth: JWKS URL must use HTTP or HTTPS")
	}
	if jwksURL.User != nil || jwksURL.Fragment != "" {
		return nil, errors.New("auth: JWKS URL must not contain credentials or a fragment")
	}

	header := strings.TrimSpace(cfg.Header)
	if header == "" {
		header = defaultAssertionHeader
	}
	if strings.ContainsAny(header, "\r\n") {
		return nil, errors.New("auth: invalid assertion header")
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}
	if cacheTTL < 0 {
		return nil, errors.New("auth: cache TTL must not be negative")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}

	health := make(map[string]struct{}, len(cfg.HealthPaths))
	for _, path := range cfg.HealthPaths {
		if path == "" || path[0] != '/' || strings.ContainsAny(path, "?#") {
			return nil, fmt.Errorf("auth: invalid health path %q", path)
		}
		health[path] = struct{}{}
	}

	return &Authenticator{
		issuer:     issuer,
		audience:   audience,
		adminEmail: adminEmail,
		jwksURL:    jwksURL,
		header:     header,
		health:     health,
		cacheTTL:   cacheTTL,
		client:     client,
		now:        now,
		keys:       make(map[string]*rsa.PublicKey),
	}, nil
}

type principalContextKey struct{}

// PrincipalFromContext returns the user authenticated by Middleware.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// Middleware requires a valid Cloudflare assertion on every request other
// than configured GET/HEAD health paths. Authentication failures intentionally
// return one generic response and never disclose validation details.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	if next == nil {
		panic("auth: nil HTTP handler")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.healthExempt(r) {
			next.ServeHTTP(w, r)
			return
		}

		token := strings.TrimSpace(r.Header.Get(a.header))
		if token == "" {
			unauthorized(w)
			return
		}
		principal, err := a.Validate(r.Context(), token)
		if err != nil {
			unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Authenticator) healthExempt(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	_, ok := a.health[r.URL.Path]
	return ok
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized\n"))
}

type jwtHeader struct {
	Algorithm string            `json:"alg"`
	KeyID     string            `json:"kid"`
	Critical  []json.RawMessage `json:"crit"`
}

type accessClaims struct {
	issuer    string
	audiences []string
	email     string
	subject   string
	typeName  string
	expiresAt time.Time
	notBefore *time.Time
}

// Validate verifies a raw Cloudflare Access assertion and returns its
// principal. The token must use RS256 and a key from the configured JWKS.
func (a *Authenticator) Validate(ctx context.Context, token string) (Principal, error) {
	if token == "" || len(token) > maxTokenBytes {
		return Principal{}, errors.New("auth: malformed token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Principal{}, errors.New("auth: malformed token")
	}

	headerBytes, err := decodeJWTPart(parts[0])
	if err != nil {
		return Principal{}, errors.New("auth: malformed token header")
	}
	var header jwtHeader
	if err := decodeJSONObject(headerBytes, &header); err != nil {
		return Principal{}, errors.New("auth: malformed token header")
	}
	if header.Algorithm != "RS256" || header.KeyID == "" || len(header.Critical) != 0 {
		return Principal{}, errors.New("auth: unsupported token header")
	}

	key, err := a.key(ctx, header.KeyID)
	if err != nil {
		return Principal{}, fmt.Errorf("auth: key lookup: %w", err)
	}
	signature, err := decodeJWTPart(parts[2])
	if err != nil {
		return Principal{}, errors.New("auth: malformed token signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Principal{}, errors.New("auth: invalid token signature")
	}

	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return Principal{}, errors.New("auth: malformed token claims")
	}
	claims, err := parseAccessClaims(payload)
	if err != nil {
		return Principal{}, err
	}
	if claims.issuer != a.issuer || !containsExact(claims.audiences, a.audience) {
		return Principal{}, errors.New("auth: token issuer or audience mismatch")
	}
	if claims.typeName != "app" {
		return Principal{}, errors.New("auth: token is not an application token")
	}
	// Cloudflare email identities are canonicalized to lower case. Requiring
	// the claim itself to be canonical avoids accepting two textual identities
	// as the configured administrator.
	if claims.email != strings.ToLower(claims.email) || claims.email != a.adminEmail {
		return Principal{}, errors.New("auth: token email mismatch")
	}

	now := a.now()
	if !now.Before(claims.expiresAt) {
		return Principal{}, errors.New("auth: token expired")
	}
	if claims.notBefore != nil && now.Before(*claims.notBefore) {
		return Principal{}, errors.New("auth: token not active")
	}

	return Principal{
		Email:     claims.email,
		Subject:   claims.subject,
		ExpiresAt: claims.expiresAt,
	}, nil
}

func parseAccessClaims(payload []byte) (accessClaims, error) {
	var raw map[string]json.RawMessage
	if err := decodeJSONObject(payload, &raw); err != nil {
		return accessClaims{}, errors.New("auth: malformed token claims")
	}

	issuer, err := requiredStringClaim(raw, "iss")
	if err != nil {
		return accessClaims{}, err
	}
	email, err := requiredStringClaim(raw, "email")
	if err != nil {
		return accessClaims{}, err
	}
	typeName, err := requiredStringClaim(raw, "type")
	if err != nil {
		return accessClaims{}, err
	}
	audiences, err := audienceClaim(raw["aud"])
	if err != nil || len(audiences) == 0 {
		return accessClaims{}, errors.New("auth: invalid aud claim")
	}
	expiresAt, err := numericDateClaim(raw, "exp", true)
	if err != nil || expiresAt == nil {
		return accessClaims{}, errors.New("auth: invalid exp claim")
	}
	notBefore, err := numericDateClaim(raw, "nbf", false)
	if err != nil {
		return accessClaims{}, errors.New("auth: invalid nbf claim")
	}

	var subject string
	if value, ok := raw["sub"]; ok {
		if err := json.Unmarshal(value, &subject); err != nil {
			return accessClaims{}, errors.New("auth: invalid sub claim")
		}
	}

	return accessClaims{
		issuer:    issuer,
		audiences: audiences,
		email:     email,
		subject:   subject,
		typeName:  typeName,
		expiresAt: *expiresAt,
		notBefore: notBefore,
	}, nil
}

func requiredStringClaim(raw map[string]json.RawMessage, name string) (string, error) {
	value, ok := raw[name]
	if !ok {
		return "", fmt.Errorf("auth: missing %s claim", name)
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil || result == "" {
		return "", fmt.Errorf("auth: invalid %s claim", name)
	}
	return result, nil
}

func audienceClaim(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("missing audience")
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, errors.New("empty audience")
		}
		return []string{single}, nil
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil {
		return nil, err
	}
	for _, audience := range multiple {
		if audience == "" {
			return nil, errors.New("empty audience")
		}
	}
	return multiple, nil
}

func numericDateClaim(raw map[string]json.RawMessage, name string, required bool) (*time.Time, error) {
	value, ok := raw[name]
	if !ok {
		if required {
			return nil, errors.New("missing numeric date")
		}
		return nil, nil
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return nil, err
	}
	seconds, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return nil, err
	}
	result := time.Unix(seconds, 0)
	return &result, nil
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func decodeJWTPart(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

func decodeJSONObject(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
