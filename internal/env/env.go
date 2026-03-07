package env

import (
	"os"
	"path/filepath"
	"strings"
)

// OPPATH returns the user-local runtime home used by op.
func OPPATH() string {
	if runtimeHome := strings.TrimSpace(os.Getenv("OPPATH")); runtimeHome != "" {
		return runtimeHome
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".op"
	}
	return filepath.Join(home, ".op")
}

// OPBIN returns the canonical install directory for holon binaries.
func OPBIN() string {
	if binaryHome := strings.TrimSpace(os.Getenv("OPBIN")); binaryHome != "" {
		return binaryHome
	}
	return filepath.Join(OPPATH(), "bin")
}

// CacheDir returns the dependency cache used by op.
func CacheDir() string {
	return filepath.Join(OPPATH(), "cache")
}

// Init creates the runtime home and binary directory if they do not exist.
func Init() error {
	for _, dir := range []string{OPPATH(), OPBIN()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ShellSnippet returns a shell fragment suitable for zsh/bash startup files.
func ShellSnippet() string {
	return strings.Join([]string{
		`export OPPATH="${OPPATH:-$HOME/.op}"`,
		`export OPBIN="${OPBIN:-$OPPATH/bin}"`,
		`mkdir -p "$OPBIN"`,
		`export PATH="$OPBIN:$PATH"`,
	}, "\n")
}
