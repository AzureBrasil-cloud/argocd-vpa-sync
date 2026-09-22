//go:build dist

// Package web embeds the dashboard's built assets into the Go binary. Build
// with `-tags dist` (see Dockerfile / `make build-dist`) after running
// `npm run build` in this directory -- plain `go build ./...` (no tags)
// intentionally does NOT require web/dist to exist, so the backend can be
// built and tested without Node installed; see embed_stub.go.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS serves the dashboard's built assets (index.html, JS/CSS bundles).
var FS fs.FS

// Embedded is true when this binary was built with the real dashboard
// assets (`-tags dist`).
const Embedded = true

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// distFS is a compile-time embed of a directory this same package
		// declares; a failure here means the embed itself is broken, which
		// should never happen for a successful build.
		panic(err)
	}
	FS = sub
}
