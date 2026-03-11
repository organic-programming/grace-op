# OP — The Organic Programming CLI

> *"One command, every holon."*

OP is the unified entry point to the Organic Programming ecosystem.
It discovers holons — locally or over the network — and dispatches
commands to them through a single interface.

## Install

```bash
GOBIN="$OPBIN" go install github.com/organic-programming/grace-op/cmd/op@latest
```

The binary is installed as `op` (not `grace-op`) because the Go module
entry point is `cmd/op`.

Package-manager install paths and uninstall commands live in
[INSTALL.md](INSTALL.md).

### From Source

```bash
git clone https://github.com/organic-programming/grace-op.git
cd grace-op
go build -o op ./cmd/op && mv op "$OPBIN/"
```

## Environment Setup

OP stores its runtime data under `$OPPATH` (defaults to `~/.op`) and
installs binaries into `$OPBIN` (defaults to `$OPPATH/bin`).

### macOS / Linux

```bash
# Create the directories
op env --init

# Append the shell snippet to your profile
op env --shell >> ~/.zshrc   # or ~/.bashrc
source ~/.zshrc
```

`op env --shell` outputs:

```bash
export OPPATH="${OPPATH:-$HOME/.op}"
export OPBIN="${OPBIN:-$OPPATH/bin}"
mkdir -p "$OPBIN"
export PATH="$OPBIN:$PATH"
```

### Windows

```powershell
# Create the directories
op env --init

# Set environment variables (persistent, user-level)
[System.Environment]::SetEnvironmentVariable("OPPATH", "$env:USERPROFILE\.op", "User")
[System.Environment]::SetEnvironmentVariable("OPBIN", "$env:USERPROFILE\.op\bin", "User")

# Add OPBIN to PATH
$path = [System.Environment]::GetEnvironmentVariable("PATH", "User")
[System.Environment]::SetEnvironmentVariable("PATH", "$env:USERPROFILE\.op\bin;$path", "User")
```

Restart your terminal after running these commands.

## Usage

```
# Identity commands
op new                               → create a new holon identity
op new --list                        → list shipped templates
op new --template go-daemon my-app   → generate a scaffold
op list                              → list all known holons
op show <uuid>                       → display a holon's identity

# Full namespace (dispatch to any holon binary)
op sophia-who list                   → direct holon dispatch
op who list                          → alias of sophia-who
op translate file.md --to fr         → abel-fishel-translator

# OP's own commands
op discover                          → list all available holons
op install my-app --no-build         → install an existing artifact
op install my-ui --link-applications → install a .app and link it on macOS
op mod pull                          → fetch dependencies into $OPPATH/cache
op env --shell                       → print shell setup
op version                           → show op version
```

## Templates

`op new --template` ships scaffold templates for daemon, wrapper,
toolchain, composition, host UI, and composite holons. Use
`op new --list` to inspect the catalog, then generate into the current
workspace:

```bash
op new --template go-daemon wisupaa-whisper
op new --template composite-go-swiftui studio-console
```

## Sophia Who? list over every transport

Use `ListIdentities` (the gRPC equivalent of `sophia-who list`) through each
transport supported by Sophia Who?:

```bash
# 1) CLI facet (delegated command)
op who list .
op sophia-who list .

# 2) Promoted verb (same provider behavior as `sophia-who list`)
op list .

# 3) gRPC over TCP (persistent server)
op run sophia-who:9090
op grpc://localhost:9090 ListIdentities '{}'
# stop with: kill <pid printed by op run>

# 4) gRPC over Unix socket (persistent server)
op run sophia-who --listen unix:///tmp/who.sock
op grpc+unix:///tmp/who.sock ListIdentities '{}'
# stop with: kill <pid printed by op run>

# 5) gRPC over stdio (ephemeral, no `op run`)
op grpc+stdio://sophia-who ListIdentities '{}'
```

## Shell Completion

`op` supports tab-completion for **zsh** and **bash**.

```bash
# zsh — add to ~/.zshrc
eval "$(op completion zsh)"

# bash — add to ~/.bashrc
eval "$(op completion bash)"
```

Then restart your shell. Completions use the existing discovery
mechanism — identity-derived slugs, OPBIN entries, and PATH binaries
are all suggested:

```
op run gudule-<TAB>       →  gudule-greeting-godart, gudule-greeting-goswift, ...
op build sophia<TAB>      →  sophia-who
op uninstall <TAB>        →  only installed holons from $OPBIN
```

## Status

v0.3.0-dev — composite artifacts, expanded runners, template scaffolds, and release packaging.

## Design Drafts

- [OP_BUILD_SPEC.md](OP_BUILD_SPEC.md) — proposed contract for
  manifest-driven `op build`, including composite holons and `recipe`
  orchestration.

## Organic Programming

This holon is part of the [Organic Programming](https://github.com/organic-programming/seed)
ecosystem. For context, see:

- [Constitution](https://github.com/organic-programming/seed/blob/master/AGENT.md) — what a holon is
