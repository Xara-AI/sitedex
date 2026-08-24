// Package version holds build-time version metadata for sitedex.
//
// Version, Commit, and Date are overridden at build time via -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/Xara-AI/sitedex/internal/version.Version=v0.1.0 \
//	  -X github.com/Xara-AI/sitedex/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/Xara-AI/sitedex/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

// Version is the sitedex release version. "dev" for local/unreleased builds.
var Version = "dev"

// Commit is the git commit hash the binary was built from.
var Commit = "none"

// Date is the UTC build timestamp, RFC3339.
var Date = "unknown"

// String returns a one-line human-readable version string, e.g.
// "sitedex dev (commit none, built unknown)".
func String() string {
	return "sitedex " + Version + " (commit " + Commit + ", built " + Date + ")"
}

// UserAgent returns the default HTTP User-Agent identifying this binary to
// crawled sites, per the politeness requirements in CLAUDE.md.
func UserAgent() string {
	return "sitedex/" + Version + " (+https://github.com/Xara-AI/sitedex)"
}
