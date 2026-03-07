package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	openv "github.com/organic-programming/grace-op/internal/env"
	"github.com/organic-programming/grace-op/internal/holons"
)

type envOutput struct {
	OPPATH      string   `json:"oppath"`
	OPBIN       string   `json:"opbin"`
	Roots       []string `json:"roots"`
	Initialized bool     `json:"initialized,omitempty"`
	Shell       string   `json:"shell,omitempty"`
}

func cmdEnv(format Format, args []string) int {
	var (
		initDirs   bool
		shell      bool
		positional []string
	)

	for _, arg := range args {
		switch arg {
		case "--init":
			initDirs = true
		case "--shell":
			shell = true
		default:
			if strings.HasPrefix(arg, "--") {
				fmt.Fprintf(os.Stderr, "op env: unknown flag %q\n", arg)
				return 1
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) > 0 {
		fmt.Fprintln(os.Stderr, "op env: does not accept positional arguments")
		return 1
	}

	if initDirs {
		if err := openv.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "op env: %v\n", err)
			return 1
		}
	}

	payload := envOutput{
		OPPATH:      openv.OPPATH(),
		OPBIN:       openv.OPBIN(),
		Roots:       holons.KnownRootLabels(),
		Initialized: initDirs,
	}
	if shell {
		payload.Shell = openv.ShellSnippet()
	}

	if format == FormatJSON {
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "op env: %v\n", err)
			return 1
		}
		fmt.Println(string(out))
		return 0
	}

	if shell {
		if initDirs {
			fmt.Fprintf(os.Stderr, "op env: initialized %s and %s\n", payload.OPPATH, payload.OPBIN)
		}
		fmt.Println(payload.Shell)
		return 0
	}

	fmt.Printf("OPPATH=%s\n", payload.OPPATH)
	fmt.Printf("OPBIN=%s\n", payload.OPBIN)
	fmt.Printf("ROOTS=%s\n", strings.Join(payload.Roots, ", "))
	if initDirs {
		fmt.Printf("INITIALIZED=%t\n", payload.Initialized)
	}
	return 0
}
