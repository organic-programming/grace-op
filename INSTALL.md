# Installing `op`

## Quick Install

### macOS

```bash
brew tap organic-programming/homebrew-tap
brew install op
```

### Windows

```powershell
winget install OrganicProgramming.Op
```

### Any platform with Node.js

```bash
npm install -g @organic-programming/op
```

### Any platform with Go

```bash
GOBIN="$OPBIN" go install github.com/organic-programming/grace-op/cmd/op@latest
```

## Verify

```bash
op version
op env
```

## From Source

```bash
git clone https://github.com/organic-programming/grace-op.git
cd grace-op
go build -o op ./cmd/op
mv op "$OPBIN/"
```

## Uninstall

### Homebrew

```bash
brew uninstall op
```

### WinGet

```powershell
winget uninstall OrganicProgramming.Op
```

### npm

```bash
npm uninstall -g @organic-programming/op
```

### Go / source installs

```bash
rm -f "$OPBIN/op"
```

## Notes

- GitHub tag builds generate Homebrew formula output, WinGet manifests, and npm package staging from `scripts/releaser.go`.
- Homebrew, WinGet, and npm commands require published release artifacts for the tagged version you want to install.
