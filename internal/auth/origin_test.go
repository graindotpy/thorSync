package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		origin  string
		referer string
		fetch   string
		want    bool
	}{
		{name: "same origin", method: http.MethodPost, origin: "https://sync.example.com", want: true},
		{name: "same origin default port", method: http.MethodPost, origin: "https://sync.example.com:443", want: true},
		{name: "cross origin host", method: http.MethodPost, origin: "https://evil.example", want: false},
		{name: "cross origin scheme", method: http.MethodPost, origin: "http://sync.example.com", want: false},
		{name: "null origin", method: http.MethodPost, origin: "null", want: false},
		{name: "same origin referer", method: http.MethodPost, referer: "https://sync.example.com/games/42", want: true},
		{name: "cross origin referer", method: http.MethodPost, referer: "https://evil.example/attack", want: false},
		{name: "same origin fetch metadata", method: http.MethodPost, fetch: "same-origin", want: true},
		{name: "same site is not same origin", method: http.MethodPost, fetch: "same-site", want: false},
		{name: "cross site fetch metadata", method: http.MethodPost, fetch: "cross-site", want: false},
		{name: "non browser client", method: http.MethodPost, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "https://sync.example.com/api/games", nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Referer", test.referer)
			request.Header.Set("Sec-Fetch-Site", test.fetch)
			if got := SameOrigin(request); got != test.want {
				t.Fatalf("SameOrigin() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSameOriginUsesForwardedProto(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://sync.example.com/api/games", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("Origin", "https://sync.example.com")
	if !SameOrigin(request) {
		t.Fatal("SameOrigin() rejected the externally visible HTTPS origin")
	}
}

func TestSameOriginMutationsRejectsCrossOriginAndDisablesCORS(t *testing.T) {
	called := 0
	handler := SameOriginMutations(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.WriteHeader(http.StatusNoContent)
	}))

	sameOrigin := httptest.NewRequest(http.MethodPost, "https://sync.example.com/api/restore", nil)
	sameOrigin.Header.Set("Origin", "https://sync.example.com")
	sameOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(sameOriginResponse, sameOrigin)
	if sameOriginResponse.Code != http.StatusNoContent {
		t.Fatalf("same-origin status = %d", sameOriginResponse.Code)
	}
	if got := sameOriginResponse.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "https://sync.example.com/api/restore", nil)
	crossOrigin.Header.Set("Origin", "https://evil.example")
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403", crossOriginResponse.Code)
	}
	if called != 1 {
		t.Fatalf("nested handler calls = %d, want 1", called)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "https://sync.example.com/api/restore", nil)
	preflight.Header.Set("Origin", "https://evil.example")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin preflight status = %d, want 403", preflightResponse.Code)
	}
}
