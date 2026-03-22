# OP — The Organic Programming CLI

> *"One command, every holon."*

OP discovers holons, builds them, manages their identities, and
dispatches commands — all from a single binary.

## Install

### macOS / Linux

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/organic-programming/grace-op/dev/scripts/install.sh)"
```

Or via Homebrew:

```bash
brew tap organic-programming/tap && brew install op
```

### Windows

```powershell
irm https://raw.githubusercontent.com/organic-programming/grace-op/dev/scripts/install.ps1 | iex
```

Or via Chocolatey:

```powershell
choco install op
```

### Go (any platform)

```bash
go install github.com/organic-programming/grace-op/cmd/op@latest
op env --init
op install .
```

Then activate (or restart your terminal):

```bash
eval "$(op env --shell)"                                    # macOS / Linux
```
```powershell
op env --init   # Windows (OPBIN is added to PATH by op install)
```

### From source

```bash
git clone https://github.com/organic-programming/grace-op.git
cd grace-op
go run ./cmd/op env --init
go run ./cmd/op install .
```

Then activate as above, or restart your terminal.

## Usage

```bash
# Create a holon
op new --template go-daemon my-service

# Build, test, install
op build my-service
op test my-service
op install my-service

# Dispatch commands
op my-service SayHello '{"name":"Alice"}'

# Direct transport
op grpc+stdio://my-service SayHello '{"name":"Alice"}'

# Discover and inspect
op list
op inspect my-service
```

## Status

v0.5 — proto-first manifest, embedded canonical protos, proto stage
pipeline, generated REFERENCE.md, full lifecycle.

## Documentation

- [OP.md](OP.md) — full specification (manifest, discovery, lifecycle, transport)
- [HOLON_BUILD.md](../../HOLON_BUILD.md) — `op build` specification
- [HOLON_PROTO.md](HOLON_PROTO.md) — proto manifest authoring guide
- [HOLON_PACKAGE.md](HOLON_PACKAGE.md) — `.holon` package format

## Organic Programming

Part of the [Organic Programming](https://github.com/organic-programming/seed)
ecosystem. See the [Constitution](https://github.com/organic-programming/seed/blob/master/AGENT.md).