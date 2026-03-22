# Installing `op`

## One-Liner Install

### macOS / Linux

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/organic-programming/grace-op/dev/scripts/install.sh)"
```

This script:
1. Detects your OS and architecture
2. Downloads the latest `op` binary
3. Creates `~/.op/bin/` and installs `op` there
4. Prints the shell snippet to add to your profile

### Windows

```powershell
irm https://raw.githubusercontent.com/organic-programming/grace-op/dev/scripts/install.ps1 | iex
```

This script:
1. Downloads the latest `op.exe`
2. Creates `%USERPROFILE%\.op\bin\` and installs `op.exe` there
3. Adds `%USERPROFILE%\.op\bin` to the user PATH

## Alternative Install Methods

### Homebrew (macOS)

```bash
brew tap organic-programming/homebrew-tap
brew install op
```

### Go

```bash
GOBIN="$OPBIN" go install github.com/organic-programming/grace-op/cmd/op@latest
```

> Requires at least one published Git tag on the grace-op repository.

### npm

```bash
npm install -g @organic-programming/op
```

### From Source

```bash
git clone https://github.com/organic-programming/grace-op.git
cd grace-op
go run ./cmd/op install .
```

This uses `op install` to build and self-install into `~/.op/bin/`.

### Self-Install (from a built binary)

If you already have an `op` binary:

```bash
op install .     # from the grace-op directory
op env --init    # create ~/.op/ directories
```

## Environment Setup

OP stores runtime data under `$OPPATH` (defaults to `~/.op`) and
installs binaries into `$OPBIN` (defaults to `$OPPATH/bin`).

### macOS / Linux

```bash
# Create the directories
op env --init

# Add to your shell profile (~/.zshrc or ~/.bashrc)
eval "$(op env --shell)"
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

### Verify

```bash
op version
op env
```

## Shell Completion

### zsh

```bash
# Add to ~/.zshrc
eval "$(op completion zsh)"
```

### bash

```bash
# Add to ~/.bashrc
eval "$(op completion bash)"
```

After sourcing, tab-completion works for holon names, commands,
and flags:

```
op run gabriel-<TAB>       →  gabriel-greeting-go, gabriel-greeting-swift, ...
op build grace<TAB>        →  grace-op
op uninstall <TAB>         →  only installed holons from $OPBIN
```

## Uninstall

### One-liner installs / Go / Source

```bash
rm -rf ~/.op
```

Or to remove just the `op` binary:

```bash
rm -f "$OPBIN/op"
```

### Homebrew

```bash
brew uninstall op
```

### npm

```bash
npm uninstall -g @organic-programming/op
```

### Windows

```powershell
Remove-Item -Recurse -Force "$env:USERPROFILE\.op"
# Remove OPBIN from PATH via System > Environment Variables
```

## Release Notes

- GitHub tag builds generate Homebrew formula output, WinGet manifests,
  and npm package staging from `scripts/releaser.go`.
- Install scripts and package-manager commands require published
  release artifacts for the version being installed.
