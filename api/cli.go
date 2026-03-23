package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	holonserve "github.com/organic-programming/go-holons/pkg/serve"
	opv1 "github.com/organic-programming/grace-op/gen/go/op/v1"
	"github.com/organic-programming/grace-op/internal/mcp"
	"github.com/organic-programming/grace-op/internal/scaffold"
	"github.com/organic-programming/grace-op/internal/server"
	"github.com/organic-programming/grace-op/internal/who"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type cliState struct {
	stdout  io.Writer
	stderr  io.Writer
	version string
}

func RunCLI(args []string, outputs ...io.Writer) int {
	return RunCLIWithVersion(args, VersionString(), outputs...)
}

func RunCLIWithVersion(args []string, version string, outputs ...io.Writer) int {
	state := cliState{
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		version: version,
	}
	if len(outputs) > 0 && outputs[0] != nil {
		state.stdout = outputs[0]
	}
	if len(outputs) > 1 && outputs[1] != nil {
		state.stderr = outputs[1]
	}
	return state.run(args)
}

func (c cliState) run(args []string) int {
	format, quiet, args, err := parseGlobalOptions(args)
	if err != nil {
		fmt.Fprintf(c.stderr, "op: %v\n", err)
		return 1
	}
	if len(args) == 0 {
		c.printUsage()
		return 1
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "check", "build", "test", "clean":
		return c.runLifecycleCommand(format, quiet, cmd, rest)
	case "install":
		return c.runInstallCommand(format, quiet, rest)
	case "uninstall":
		return c.runUninstallCommand(format, quiet, rest)
	case "mod":
		return c.runModCommand(format, quiet, rest)
	case "run":
		return c.runRunCommand(format, quiet, rest)
	case "discover":
		return c.runDiscoverCommand(format, rest)
	case "inspect":
		return c.runInspectCommand(format, rest)
	case "do":
		return c.runSequenceCommand(format, rest)
	case "mcp":
		return c.runMCPCommand(rest)
	case "tools":
		return c.runToolsCommand(rest)
	case "env":
		return c.runEnvCommand(format, rest)
	case "serve":
		return c.runServeCommand(rest)
	case "version":
		fmt.Fprintf(c.stdout, "op %s\n", c.version)
		return 0
	case "completion":
		return c.runCompletionCommand(rest)
	case "__complete":
		return c.runCompleteCommand(rest)
	case "help", "--help", "-h":
		c.printUsage()
		return 0
	case "new", "list", "show":
		return c.runIdentityCommand(format, quiet, cmd, rest)
	default:
		if strings.HasPrefix(cmd, "grpc://") ||
			strings.HasPrefix(cmd, "grpc+tcp://") ||
			strings.HasPrefix(cmd, "grpc+stdio://") ||
			strings.HasPrefix(cmd, "grpc+unix://") ||
			strings.HasPrefix(cmd, "grpc+ws://") ||
			strings.HasPrefix(cmd, "grpc+wss://") {
			return c.runGRPCCommand(format, cmd, rest)
		}
		return c.runHolonCommand(format, cmd, rest)
	}
}

func (c cliState) printUsage() {
	fmt.Fprint(c.stdout, `op — the Organic Programming CLI

Global flags (must come before <holon> or URI):
  -f, --format <text|json>              output format for RPC responses (default: text)
  -q, --quiet                           suppress progress output

Holon dispatch (transport chain):
  op <holon> <command> [args]            dispatch via the SDK auto-connect chain

Direct gRPC URI dispatch:
  op grpc://<slug|host:port> <method>     gRPC auto-connect for slugs, direct TCP for host:port
  op grpc+tcp://<slug|host:port> <method> force gRPC over TCP
  op grpc+stdio://<holon> <method>        force gRPC over stdio pipe
  op grpc+unix://<path> <method>          gRPC over Unix socket
  op grpc+ws://<host:port> <method>       gRPC over WebSocket
  op grpc+wss://<host:port> <method>      gRPC over secure WebSocket

OP commands:
  op list [root]
  op show <uuid-or-prefix>
  op new [--json <payload>]
  op new --list
  op new --template <name> <holon-name> [--set key=value]
  op discover
  op inspect <slug|host:port> [--json]
  op do <holon> <sequence> [--param=value ...]
  op tools <slug> [--format <fmt>]
  op check [<holon-or-path>]
  op build [<holon-or-path>] [flags]
  op test [<holon-or-path>]
  op clean [<holon-or-path>]
  op run <holon> [flags]
  op install [<holon-or-path>] [flags]
  op uninstall <holon>
  op mod <command>
  op env [--init] [--shell]
  op mcp <slug> [slug2...]
  op mcp <grpc+tcp://host:port>
  op serve [--listen tcp://:9090]
  op version
`)
}

func parseGlobalOptions(args []string) (Format, bool, []string, error) {
	format := FormatText
	quiet := false
	i := 0
	for i < len(args) {
		switch {
		case args[i] == "--quiet" || args[i] == "-q":
			quiet = true
			i++
		case args[i] == "--format" || args[i] == "-f":
			if i+1 >= len(args) {
				return "", false, nil, fmt.Errorf("%s requires a value (text or json)", args[i])
			}
			parsed, err := parseFormat(args[i+1])
			if err != nil {
				return "", false, nil, err
			}
			format = parsed
			i += 2
		case strings.HasPrefix(args[i], "--format="):
			parsed, err := parseFormat(strings.TrimPrefix(args[i], "--format="))
			if err != nil {
				return "", false, nil, err
			}
			format = parsed
			i++
		case strings.HasPrefix(args[i], "-f="):
			parsed, err := parseFormat(strings.TrimPrefix(args[i], "-f="))
			if err != nil {
				return "", false, nil, err
			}
			format = parsed
			i++
		default:
			return format, quiet, args[i:], nil
		}
	}
	return format, quiet, nil, nil
}

func parseFormat(value string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(value))) {
	case FormatText:
		return FormatText, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid --format %q (supported: text, json)", value)
	}
}

func (c cliState) writeFormatted(format Format, resp proto.Message) {
	if resp == nil {
		return
	}
	out := strings.TrimSpace(FormatResponse(format, resp))
	if out != "" {
		fmt.Fprintln(c.stdout, out)
	}
}

func printJSON(w io.Writer, value any) error {
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", out)
	return err
}

func (c cliState) runMCPCommand(args []string) int {
	// Extract --listen flag if present.
	var listenAddr string
	var slugs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--listen" {
			if i+1 >= len(args) {
				fmt.Fprintln(c.stderr, "op mcp: --listen requires a value")
				return 1
			}
			listenAddr = args[i+1]
			i++
		} else {
			slugs = append(slugs, args[i])
		}
	}

	if len(slugs) == 0 {
		fmt.Fprintln(c.stderr, "op mcp: requires at least one <slug> or URI")
		return 1
	}

	var serverInstance *mcp.Server
	var err error

	if len(slugs) == 1 && mcp.IsURI(slugs[0]) {
		serverInstance, err = mcp.NewServerFromURI(slugs[0], c.version)
	} else {
		serverInstance, err = mcp.NewServer(slugs, c.version)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "op mcp: %v\n", err)
		return 1
	}
	defer func() { _ = serverInstance.Close() }()

	if listenAddr != "" {
		addr := mcp.ParseHTTPListenAddr(listenAddr)
		if err := serverInstance.ServeHTTP(addr); err != nil {
			fmt.Fprintf(c.stderr, "op mcp: %v\n", err)
			return 1
		}
		return 0
	}

	if err := serverInstance.ServeStdio(context.Background(), os.Stdin, c.stdout); err != nil {
		fmt.Fprintf(c.stderr, "op mcp: %v\n", err)
		return 1
	}
	return 0
}

func (c cliState) runServeCommand(args []string) int {
	options := holonserve.ParseOptions(args)
	if err := holonserve.RunWithOptions(options.ListenURI, func(s *grpc.Server) {
		server.Register(s, RPCHandler{})
	}, options.Reflect); err != nil {
		fmt.Fprintf(c.stderr, "op serve: %v\n", err)
		return 1
	}
	return 0
}

func flagValue(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func flagOrDefault(args []string, key, defaultVal string) string {
	if value := flagValue(args, key); value != "" {
		return value
	}
	return defaultVal
}

func emitQuietNote(_ bool) {}

func templateEntriesFromResponse(resp *opv1.ListTemplatesResponse) []scaffold.Entry {
	entries := make([]scaffold.Entry, 0, len(resp.GetEntries()))
	for _, entry := range resp.GetEntries() {
		params := make([]scaffold.Param, 0, len(entry.GetParams()))
		for _, param := range entry.GetParams() {
			params = append(params, scaffold.Param{
				Name:        param.GetName(),
				Description: param.GetDescription(),
				Default:     param.GetDefault(),
				Required:    param.GetRequired(),
			})
		}
		entries = append(entries, scaffold.Entry{
			Name:        entry.GetName(),
			Description: entry.GetDescription(),
			Lang:        entry.GetLang(),
			Params:      params,
		})
	}
	return entries
}

func createInteractive(out io.Writer) (*opv1.CreateIdentityResponse, error) {
	return who.CreateInteractive(os.Stdin, out)
}

func createFromJSON(raw string) (*opv1.CreateIdentityResponse, error) {
	return who.CreateFromJSON(raw)
}
