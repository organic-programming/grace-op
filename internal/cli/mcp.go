package cli

import (
	"context"
	"fmt"
	"os"

	mcppkg "github.com/organic-programming/grace-op/internal/mcp"
)

func cmdMCP(args []string, version string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "op mcp: requires at least one <slug>")
		return 1
	}

	server, err := mcppkg.NewServer(args, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "op mcp: %v\n", err)
		return 1
	}
	defer func() { _ = server.Close() }()

	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "op mcp: %v\n", err)
		return 1
	}
	return 0
}
