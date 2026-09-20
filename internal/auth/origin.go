package auth

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SameOrigin reports whether the browser-provided origin information is
// compatible with the request target. Requests from non-browser clients which
// provide none of Origin, Referer, or Sec-Fetch-Site are allowed. The Host
// header is authoritative; X-Forwarded-Host is intentionally ignored.
func SameOrigin(r *http.Request) bool {
	if r == nil {
		return false
	}

	requestScheme := requestURLScheme(r)
	requestHost, ok := canonicalAuthority(r.Host, requestScheme)
	if !ok {
		return false
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return originMatches(origin, requestScheme, requestHost, false)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return originMatches(referer, requestScheme, requestHost, true)
	}

	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "cross-site", "same-site":
		return false
	case "same-origin", "none", "":
		return true
	default:
		return false
	}
}

// SameOriginMutations rejects cross-origin browser requests and applies a
// same-origin check to every state-changing HTTP method. It also strips any
// Access-Control-* response headers, keeping CORS disabled even if a nested
// handler accidentally adds them.
func SameOriginMutations(next http.Handler) http.Handler {
	if next == nil {
		panic("auth: nil HTTP handler")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originSupplied := strings.TrimSpace(r.Header.Get("Origin")) != ""
		if (originSupplied || isMutation(r.Method)) && !SameOrigin(r) {
			forbiddenOrigin(w)
			return
		}

		wrapped := &corsBlockingResponseWriter{ResponseWriter: w}
		defer stripCORSHeaders(wrapped.Header())
		next.ServeHTTP(wrapped, r)
	})
}

func isMutation(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func forbiddenOrigin(w http.ResponseWriter) {
	stripCORSHeaders(w.Header())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("cross-origin request rejected\n"))
}

func requestURLScheme(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		return forwarded
	}
	if r.URL != nil && (r.URL.Scheme == "http" || r.URL.Scheme == "https") {
		return r.URL.Scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func originMatches(raw, requestScheme, requestHost string, allowPath bool) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if !allowPath && parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	originHost, ok := canonicalAuthority(parsed.Host, parsed.Scheme)
	return ok && parsed.Scheme == requestScheme && originHost == requestHost
}

func canonicalAuthority(authority, scheme string) (string, bool) {
	authority = strings.TrimSpace(strings.ToLower(authority))
	if authority == "" || strings.ContainsAny(authority, "/\\@") {
		return "", false
	}

	var host, port string
	if strings.HasPrefix(authority, "[") && strings.HasSuffix(authority, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(authority, "["), "]")
	} else {
		var err error
		host, port, err = net.SplitHostPort(authority)
		if err != nil {
			// A missing port is expected for DNS names and IPv4 addresses.
			// Unbracketed IPv6 and malformed bracket forms remain invalid.
			if strings.Contains(authority, ":") {
				return "", false
			}
			host = authority
			port = ""
		}
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", false
	}
	isIPv6 := strings.Contains(host, ":")
	if isIPv6 {
		ip := net.ParseIP(host)
		if ip == nil {
			return "", false
		}
		host = ip.String()
	}
	if port != "" {
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return "", false
		}
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port == "" {
		if isIPv6 {
			return "[" + host + "]", true
		}
		return host, true
	}
	return net.JoinHostPort(host, port), true
}

type corsBlockingResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *corsBlockingResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	stripCORSHeaders(w.Header())
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *corsBlockingResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		stripCORSHeaders(w.Header())
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(data)
}

func (w *corsBlockingResponseWriter) Flush() {
	if !w.wroteHeader {
		stripCORSHeaders(w.Header())
		w.wroteHeader = true
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *corsBlockingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func stripCORSHeaders(header http.Header) {
	for name := range header {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			header.Del(name)
		}
	}
}
