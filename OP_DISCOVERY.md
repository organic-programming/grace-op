# `op` Discovery

Complement to [DISCOVERY.md](../../DISCOVERY.md) — the universal spec.
This document covers `op`-specific CLI behavior.

---

## Discovery Flags

| Flag | Scope | Notes|
|------|-------|-------|
| *(none)* | same as `--all` | |
| `--all` | everything across all layers | |
| `--siblings` | e.g. bundle for app's bundles | |
| `--cwd` | the execution directory | |
| `--source` | source holons in workspace | |
| `--built` | `.op/build/` packages | |
| `--installed` | `$OPBIN` packages | |
| `--cached` | `$OPPATH/cache` packages | |
| `--root <path>` | override scan root | **preempts any other scoping flag** |

---

## Working Samples

```bash
# List all discovered holons across all layers
op list

# List only source holons and installed packages
op list --source --installed

# Run a holon, forcing a scan from a specific root
op run gabriel-greeting-go --root /path/to/my/app

# Ensure resolution prefers locally built packages before checking installed ones
# (Order of flags does not matter; layer priority is fixed)
op inspect gabriel-greeting-go --built --installed
```

---

## Command Special Cases

```shell
op build    → Discover(holon, root, --source)
op install  → Discover(holon, root, --built)
op run      → Discover(holon, root, --installed, --built, --siblings)
#               ↳ if only source found → auto-build, then run
```

1. **`op build <holon>`** — forces `--source`, ignores other specifiers. `<holon>` can be a path to a source holon. If `--root` is set, builds within that root (if it contains sources recursively).
2. **`op install <holon> --build`** — composition: `build --source` then `install --installed`. Without `--build`, uses the already-built binary.
3. **`op run <holon>`** — installed → built → auto-build fallback. Add `--build` to force a build. When only a source holon is found, auto-build kicks in.

---

## Commands That Use Discovery

Every command accepts `<holon>` — any identity key (slug, alias, uuid, path, binary path).

| Command | Notes |
|---|---|
| `op <holon> <command> [args]` | dispatch via auto-connect chain |
| `op run <holon>` | |
| `op build [<holon>]` | forces `--source` specifier |
| `op check [<holon>]` | |
| `op test [<holon>]` | |
| `op clean [<holon>]` | |
| `op install [<holon>]` | |
| `op uninstall <holon>` | |
| `op do <holon> <sequence>` | |
| `op tools <holon>` | |
| `op mcp <holon>` | also accepts URI |
| `op show <holon>` | |
| `op inspect <holon>` | also accepts `host:port`[^1] |

### Exceptions

**`op list [root]`** — the positional argument is a *directory to scan*, not a `<holon>` to resolve. It answers "what's here?" not "where is X?".

**`op inspect <holon>`** — also accepts bare `host:port`, which is not a holon identity key but a network address[^1].

### No discovery

| Command | Notes |
|---|---|
| `op <binary-path> <method>` | direct file, no resolution |
| `op grpc://...` | direct URI |
| `op serve`, `op version`, `op new`, `op env`, `op mod` | self or scaffolding |

[^1]: `host:port` could be treated as a special identity key or kept as an `inspect`-only exception.

---

## Binary Path Dispatch

- `op <holon> <command> [args]` — `<holon>` can be a binary path, bypassing discovery (faster).
- Autocompletion and internal logic should cache resolved binary paths to avoid repeated discovery.
- `op <binary-path> Describe` — get the description directly, no discovery needed.

---

## The `--origin` Flag

> Replaces the former `--bin` flag.

**VERY IMPORTANT** — `op <holon> <command> --origin` shows the origin (resolved path, layer) in stderr. Operational during build.