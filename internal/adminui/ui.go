// Package adminui embeds and serves the admin console single-page app.
package adminui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var files embed.FS

// Handler serves the admin console SPA. It is served unauthenticated because the
// app itself performs login and calls protected API endpoints; the page contains
// no privileged data.
func Handler() http.Handler {
	sub, _ := fs.Sub(files, ".")
	return http.FileServer(http.FS(sub))
}
