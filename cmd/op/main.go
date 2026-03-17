package main

import (
	"os"

	"github.com/organic-programming/grace-op/api"
)

// version is set at build time by `op build` via -ldflags.
var version string

func main() {
	if version != "" {
		api.Version = version
	}
	os.Exit(api.RunCLI(os.Args[1:]))
}
