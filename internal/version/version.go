// Package version holds build-time version metadata, overridable via
// -ldflags "-X .../internal/version.Version=..." at build time.
package version

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)
