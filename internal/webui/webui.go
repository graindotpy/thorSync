package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist is replaced with the production Vite build before the Go binary is
// compiled in Docker. The tracked placeholder keeps local Go tooling usable.
//
//go:embed dist/*
var content embed.FS

func Handler() http.Handler {
	dist, _ := fs.Sub(content, "dist")
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested == "." || requested == "" {
			requested = "index.html"
		}
		if _, err := fs.Stat(dist, requested); err != nil {
			clone := r.Clone(r.Context())
			clone.URL.Path = "/index.html"
			files.ServeHTTP(w, clone)
			return
		}
		files.ServeHTTP(w, r)
	})
}
