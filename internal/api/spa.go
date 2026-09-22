package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves static files from fsys, falling back to index.html for
// any path that doesn't correspond to a real file. This is what lets the
// dashboard's client-side routes (e.g. /recommendations/{ns}/{vpa}/{container})
// survive a full page load or refresh: the browser requests that literal
// path, there's no such file in the build output, so the SPA's own router
// (react-router) is served index.html and takes over from there.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" || clean == "." {
			clean = "index.html"
		}

		if _, err := fs.Stat(fsys, clean); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
