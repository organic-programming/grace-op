package api

import (
	"fmt"
	"os"
	"strings"

	opv1 "github.com/organic-programming/grace-op/gen/go/op/v1"
)

func (c cliState) runLifecycleCommand(format Format, quiet bool, operation string, args []string) int {
	_ = quiet

	var build opv1.BuildOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--target" && i+1 < len(args):
			build.Target = args[i+1]
			i++
		case args[i] == "--mode" && i+1 < len(args):
			build.Mode = args[i+1]
			i++
		case args[i] == "--dry-run":
			build.DryRun = true
		case args[i] == "--no-sign" && operation == "build":
			build.NoSign = true
		case strings.HasPrefix(args[i], "--"):
			fmt.Fprintf(c.stderr, "op %s: unknown flag %q\n", operation, args[i])
			return 1
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		fmt.Fprintf(c.stderr, "op %s: accepts at most one <holon-or-path>\n", operation)
		return 1
	}
	target := "."
	if len(positional) == 1 {
		target = positional[0]
	}

	req := &opv1.LifecycleRequest{Target: target, Build: &build}
	var (
		resp *opv1.LifecycleResponse
		err  error
	)
	switch operation {
	case "check":
		resp, err = Check(req)
	case "build":
		resp, err = Build(req)
	case "test":
		resp, err = Test(req)
	case "clean":
		resp, err = Clean(req)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "op %s: %v\n", operation, err)
		if resp != nil && format == FormatJSON {
			c.writeFormatted(format, resp)
		}
		return 1
	}
	c.writeFormatted(format, resp)
	return 0
}

func (c cliState) runInstallCommand(format Format, quiet bool, args []string) int {
	_ = quiet
	var (
		req        opv1.InstallRequest
		positional []string
	)
	for _, arg := range args {
		switch arg {
		case "--build":
			req.Build = true
		case "--link-applications":
			req.LinkApplications = true
		default:
			if strings.HasPrefix(arg, "--") {
				fmt.Fprintf(c.stderr, "op install: unknown flag %q\n", arg)
				return 1
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		fmt.Fprintln(c.stderr, "op install: accepts at most one <holon-or-path>")
		return 1
	}
	req.Target = "."
	if len(positional) == 1 {
		req.Target = positional[0]
	}
	resp, err := Install(&req)
	if err != nil {
		fmt.Fprintf(c.stderr, "op install: %v\n", err)
		if resp != nil && format == FormatJSON {
			c.writeFormatted(format, resp)
		}
		return 1
	}
	c.writeFormatted(format, resp)
	return 0
}

func (c cliState) runUninstallCommand(format Format, quiet bool, args []string) int {
	_ = quiet
	if len(args) != 1 {
		fmt.Fprintln(c.stderr, "op uninstall: requires <holon>")
		return 1
	}
	resp, err := Uninstall(&opv1.UninstallRequest{Target: args[0]})
	if err != nil {
		fmt.Fprintf(c.stderr, "op uninstall: %v\n", err)
		if resp != nil && format == FormatJSON {
			c.writeFormatted(format, resp)
		}
		return 1
	}
	c.writeFormatted(format, resp)
	return 0
}

type runOptions struct {
	ListenURI string
	NoBuild   bool
	Target    string
	Mode      string
}

func (c cliState) runRunCommand(_ Format, quiet bool, args []string) int {
	_ = quiet
	holonName, opts, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintf(c.stderr, "op run: %v\n", err)
		return 1
	}
	req := resolveRunRequest(holonName, opts.ListenURI, opts.NoBuild, opts.Target, opts.Mode)
	resp, err := runWithIO(req, runIO{
		stdin:         os.Stdin,
		stdout:        c.stdout,
		stderr:        c.stderr,
		forwardSignal: true,
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "op run: %v\n", err)
		return 1
	}
	return int(resp.GetExitCode())
}

func parseRunArgs(args []string) (string, runOptions, error) {
	opts := runOptions{ListenURI: "stdio://"}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--listen":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--listen requires a value")
			}
			opts.ListenURI = args[i+1]
			i++
		case args[i] == "--no-build":
			opts.NoBuild = true
		case args[i] == "--target":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--target requires a value")
			}
			opts.Target = args[i+1]
			i++
		case args[i] == "--mode":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--mode requires a value")
			}
			opts.Mode = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--"):
			return "", opts, fmt.Errorf("unknown flag %q", args[i])
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return "", opts, fmt.Errorf("accepts exactly one <holon>")
	}
	return positional[0], opts, nil
}
