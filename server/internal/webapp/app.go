// Package webapp embeds and serves the listener-facing web application.
//
// It is intentionally dependency-free (no bundler, no node_modules): the whole
// client ships as a single embedded document served by the Go binary, which
// keeps the dev loop and the deployment artifact identical. The production
// mobile client (Flutter) consumes the same /api surface.
package webapp

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html
var files embed.FS

// Handler serves the listener web app. Unknown paths fall back to index.html so
// client-side routing works; the page itself holds no privileged data.
func Handler() http.Handler {
	sub, _ := fs.Sub(files, ".")
	fileServer := http.FileServer(http.FS(sub))
	index, _ := files.ReadFile("index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			serveIndex(w, index)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			serveIndex(w, index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	_, _ = w.Write(index)
}
