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
op my-service SayHello '{"name":"Bob"}'

# Direct transport
op grpc+stdio://my-service SayHello '{"name":"Bob"}'

# Discover and inspect
op list
op inspect my-service
```

## Status

v0.5 — proto-first manifest, embedded canonical protos, proto stage
pipeline, generated REFERENCE.md, full lifecycle.

## Release

Pushing a `v*` tag triggers the release pipeline (any branch):

```bash
git tag v0.5.0
git push origin v0.5.0
```

The pipeline (`go run ./scripts/releaser.go`) cross-compiles `op` for
5 targets, then GitHub Actions publishes everything:

| Target | Archive |
|--------|---------|
| darwin/amd64 | `.tar.gz` |
| darwin/arm64 | `.tar.gz` |
| linux/amd64 | `.tar.gz` |
| linux/arm64 | `.tar.gz` |
| windows/amd64 | `.zip` |

Outputs:
- **GitHub Release** — binaries, archives, SHA256 checksums
- **Homebrew tap** — auto-updated formula in `organic-programming/homebrew-tap`
- **npm** — platform-specific `@organic-programming/op-*` packages
- **WinGet** — manifests for `OrganicProgramming.Op`
- **`release.json`** — machine-readable manifest with all checksums

## Documentation

- [OP.md](OP.md) — full documentation (manifest, discovery, lifecycle, transport)
- [HOLON_DISCOVERY.md](./HOLON_DISCOVERY) 
- [HOLON_BUILD.md](./HOLON_BUILD.md) — `op build` specification
- [HOLON_PROTO.md](./HOLON_PROTO.md) — proto manifest authoring guide
- [HOLON_PACKAGE.md](./HOLON_PACKAGE.md) — `.holon` package format

## Organic Programming

Part of the [Organic Programming](https://github.com/organic-programming/seed)
ecosystem. See the [Constitution](https://github.com/organic-programming/seed/blob/master/AGENT.md).