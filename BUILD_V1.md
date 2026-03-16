# BUILD_V1 - Unified Build Artifacts for Proto-Native Holons

Status: draft

Audience:
- `grace-op` implementers
- SDK implementers
- holon authors
- composite-app recipe authors

## Why This Spec Exists

The current system still mixes three different ideas:

- source manifests used for build and discovery
- installed artifacts used for launch
- runtime metadata used for inspect and tooling

That split breaks down once holons become proto-native and source-free after
build. A built holon must remain usable:

- before install
- after install
- when embedded inside a composite app
- when referenced from an installed location

This spec defines one artifact model that works across all four cases.

## Core Position

`op build` produces self-describing artifacts.

Source manifests stay in source trees. Built artifacts do not depend on
`holon.yaml` or local proto files at runtime. Instead:

- source truth is `holon.proto` with `option (holons.v1.manifest)`
- native/service holons build into `.holon` packages
- bundle artifacts such as `.app` keep their native bundle shape
- every built artifact has a fast JSON manifest sidecar
- rich introspection comes from runtime `HolonMeta.Describe`, backed by
  metadata embedded in the built binary

## Design Goals

- One artifact contract from build output to install output
- No YAML requirement for built/runtime artifacts
- Fast local discovery without launching processes
- Rich `inspect`, `tools`, and `mcp` after launch
- Composite recipes can either embed built child holons or reference installed
  ones
- No special-casing Gabriel, Go, or Swift in the artifact model

## Non-Goals

- Replacing source manifests for `op build`
- Registry/publish protocol
- Notarization or release signing policy
- Offline rich introspection without launching the artifact
- Full migration away from legacy YAML in one step

## Truth Boundaries

### 1. Source Truth

Source holons are defined by:

- `holon.proto` with `(holons.v1.manifest)`

This is the canonical authoring format for build configuration, identity,
skills, sequences, and recipe data.

### 2. Build Truth

`op build` reads source truth and produces a build artifact plus a fast
artifact manifest.

### 3. Install Truth

`op install` copies built artifacts into `OPBIN` without changing their
internal structure.

### 4. Runtime Truth

`op inspect`, `op tools`, and `op mcp` obtain rich metadata from
`HolonMeta.Describe`, not from source files and not from the fast artifact
manifest.

## Artifact Shapes

### Native / Service Holons

Canonical build artifact:

```text
<artifact-root>/<name>.holon/
  <name>
  manifest.json
```

Rules:

- `<name>.holon` is the primary artifact
- `<name>` is the executable entrypoint
- `manifest.json` is the fast artifact manifest
- the executable must be runnable directly from inside the `.holon` directory

Example:

```text
build/gabriel-greeting-go.holon/
  gabriel-greeting-go
  manifest.json
```

### Bundle Artifacts (`.app`, future `.framework`, etc.)

Canonical build artifact:

```text
<artifact-root>/<App>.app
<artifact-root>/<App>.app.manifest.json
```

Rules:

- keep native bundle layout unchanged
- keep manifest sidecar outside the bundle
- the sidecar travels with the bundle during build, install, and publish
- external sidecar avoids invalidating code signing when metadata is written or
  updated

### Composite Artifacts

A composite holon may produce:

- a native/service `.holon` artifact
- a bundle artifact such as `.app`
- another declared primary artifact

If a composite embeds child holons, it embeds their built `.holon` packages,
not bare binaries.

## `manifest.json` Contract

`manifest.json` is JSON and intentionally small.

It is the fast local metadata file for:

- `op discover`
- shell completion
- installed resolution
- uninstall
- composite artifact wiring

Canonical schema id:

```json
{
  "schema": "op-artifact-manifest/v1"
}
```

Required fields:

```json
{
  "schema": "op-artifact-manifest/v1",
  "slug": "gabriel-greeting-go",
  "uuid": "...",
  "identity": {
    "given_name": "Gabriel",
    "family_name": "Greeting-Go",
    "motto": "..."
  },
  "lang": "go",
  "clade": "deterministic/pure",
  "status": "draft",
  "kind": "native",
  "transport": "stdio",
  "artifact": {
    "type": "holon",
    "name": "gabriel-greeting-go.holon",
    "entrypoint": "gabriel-greeting-go"
  }
}
```

For `.app` artifacts:

```json
{
  "schema": "op-artifact-manifest/v1",
  "slug": "gabriel-greeting-app-swiftui",
  "uuid": "...",
  "identity": {
    "given_name": "Gabriel",
    "family_name": "Greeting-App-SwiftUI",
    "motto": "SwiftUI HostUI for the Gabriel greeting service."
  },
  "lang": "swift",
  "clade": "deterministic/pure",
  "status": "draft",
  "kind": "composite",
  "transport": "",
  "artifact": {
    "type": "app",
    "name": "GabrielGreetingApp.app"
  }
}
```

Rules:

- `manifest.json` is not a copy of the full holon manifest
- readers must ignore unknown fields
- `slug` and `uuid` are the main resolution keys
- `manifest.json` must be cheap to read without proto parsing

## Why `manifest.json` and not `.hm`

Use `manifest.json` as the canonical filename.

Reasons:

- explicit and readable
- no custom extension hiding JSON content
- no conflict with JS packaging because the reserved filename is exact:
  - inside `.holon`: `manifest.json`
  - beside bundles: `<App>.app.manifest.json`
- `grace-op` does not treat arbitrary JSON files as artifact manifests

## Runtime Describe Contract

Rich introspection does not come from `manifest.json`.

It comes from:

- `holonmeta.v1.HolonMeta/Describe`

Protocol direction:

- keep the current service name and RPC path
- extend `DescribeResponse` compatibly
- preserve existing `slug`, `motto`, and `services`
- add a manifest-bearing field for normalized manifest data

Target semantics:

- `manifest.json` answers "what artifact is this?"
- `Describe` answers "what holon is this and what API does it expose?"

## SDK Contract

Built holons must answer `Describe` without depending on source files in cwd.

Required runtime behavior:

- build generates a complete `DescribeResponse` snapshot from source proto and
  manifest
- build embeds that snapshot into the binary
- SDK `serve` registers `HolonMeta` from embedded metadata
- source-tree parsing remains a development fallback only

This applies at least to:

- Go SDK
- Swift SDK

## CLI Semantics

### `op build`

For native/service holons:

- produces `<name>.holon/`
- reports the `.holon` directory as the primary artifact

For bundle holons:

- produces the bundle plus sibling manifest sidecar
- reports the bundle path as the primary artifact

### `op install`

For `.holon` artifacts:

- copies the entire `.holon` directory into `OPBIN`

Installed shape:

```text
OPBIN/<name>.holon/
  <name>
  manifest.json
```

For `.app` artifacts:

```text
OPBIN/<App>.app
OPBIN/<App>.app.manifest.json
```

Install is primarily a copy operation, not a repackaging step.

### `op discover`

Discovery order:

1. source holons in known roots
2. cached holons
3. built/installed artifacts with `manifest.json`
4. legacy installed binaries as fallback

For artifact discovery, `grace-op` reads only `manifest.json`.

### `op run`

`op run <slug>` may resolve to:

- a source holon
- a built `.holon` artifact
- an installed `.holon` artifact
- a top-level `.app` bundle

For `.holon`, the launch target is `<container>/<entrypoint>`.

### `op inspect`, `op tools`, `op mcp`

For artifact-backed holons:

1. resolve artifact via `manifest.json`
2. launch the artifact if needed
3. call `HolonMeta.Describe`

These commands must not require source proto files beside the built artifact.

## Recipe / Composite Model

The recipe runner must understand artifacts, not just files.

Existing step kinds remain:

- `build_member`
- `exec`
- `copy`
- `assert_file`

BUILD_V1 adds explicit artifact-aware steps:

### `copy_artifact`

Copies a member artifact or installed artifact as an artifact unit.

Examples:

- copying a `.holon` package into an app's resources
- copying a built bundle into a packaging directory

Semantics:

- if the source artifact is a directory artifact, copy the whole directory
- if the source artifact has a sibling manifest sidecar, copy that sidecar too
  when appropriate

### `use_installed`

Resolves an installed artifact from `OPBIN` and binds it to an alias for later
artifact steps.

Example shape:

```yaml
- use_installed:
    ref: gabriel-greeting-go
    as: greetingd
- copy_artifact:
    from: greetingd
    to: MyApp.app/Contents/Resources/Holons/gabriel-greeting-go.holon
```

### `build_member`

Still builds a source member, but now its output is an artifact handle, not
just an incidental path.

Example:

```yaml
- build_member: daemon
- copy_artifact:
    from: daemon
    to: MyApp.app/Contents/Resources/Holons/gabriel-greeting-go.holon
```

## Composite Policy

Both composite patterns are first-class:

- embed built child package
- reference installed holon

The spec does not force one global policy.

Recommended defaults:

- use embedded child packages for portable standalone apps
- use installed references for local developer setups, slim host apps, or
  shared system services

## Compatibility

- source `holon.yaml` remains a legacy fallback during migration
- YAML is not required by BUILD_V1 artifacts
- old bare installed binaries remain runnable as fallback
- old `Describe` clients remain valid because response growth is additive
- HostUIs should no longer need to stage proto trees for built Go/Swift holons
  once SDK embedding is implemented

## Acceptance Criteria

- native/service holons build into `.holon` packages
- bundle artifacts emit sibling `*.manifest.json`
- `op install` copies artifacts without repackaging
- `op discover` reads artifact manifests quickly
- built Go/Swift holons answer `Describe` with no source files nearby
- `op inspect`, `op tools`, and `op mcp` work against built/installed artifacts
- composite recipes can embed child `.holon` artifacts
- composite recipes can explicitly reference installed holons

## Defaults Chosen

- canonical native/service build artifact: `.holon`
- fast artifact metadata file: `manifest.json`
- rich metadata path: runtime `HolonMeta.Describe`
- Describe evolution strategy: extend current `holonmeta.v1`
- composite artifact syntax: explicit artifact-aware recipe steps
