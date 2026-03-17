package api

import "strings"

// Version is set at build time by op build via -ldflags.
// When empty (e.g. running from `go run`), resolvedVersion() tries
// to read the version from the holon's own proto at runtime.
var Version string

var (
	// Commit is injected at build time via -ldflags.
	Commit = "unknown"
)

func VersionString() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "0.0.0"
	}
	commit := strings.TrimSpace(Commit)
	if commit == "" || commit == "unknown" {
		return version
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return version + " (" + commit + ")"
}

func Banner() string {
	return "op " + VersionString()
}
