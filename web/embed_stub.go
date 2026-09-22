//go:build !dist

// Package web is the default (non-embedded) build of this package: FS is
// nil, so internal/api/server.go serves a short explanatory message at "/"
// instead of the dashboard. Build with `-tags dist` to embed the real
// dashboard build; see embed_dist.go.
package web

import "io/fs"

// FS is nil in this build; see embed_dist.go for the `-tags dist` build.
var FS fs.FS

// Embedded is false in this build.
const Embedded = false
