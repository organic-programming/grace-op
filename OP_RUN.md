⚠️ Agents must not read this document without an explicit invitation!

# `op run`
This file is a quick book for human developers and testers.

Canonical form : `op run <holon> [flags]`

## FOUND BUGS 
1. 🐞  Default `op run gabriel-greeting-go` should be equal `op run gabriel-greeting-go --listen tcp://127.0.0.1:0` Currently : `op run gabriel-greeting-go` == `op run gabriel-greeting-go --listen stdio://`
2. 📣 `op run gabriel-greeting-go --listen stdio://` stdio in this run context only should be rejected as it is absurd.
3. 🐞  `cd ~/Desktop/ && op list --root ~/Desktop/templates` is able to find an holon in `~/Desktop/isolated`
4. 🐞 IMPORTANT BUG : `op run gabriel-greeting-go --listen tcp://127.0.0.1:0 --root ~/Desktop/templates`->  `~/Desktop/isolated/ 00:00:00 ✗ run failed op run: no holon.proto found in /Users/bpds/Desktop/isolated/gabriel-greeting-go.holon`
5. 🐞 `op run gabriel-greeting-go --listen tcp://127.0.0.1:0 --bin` (fails with --bin) -> `op: holon "run" not found`  while `op gabriel-greeting-go SayHello {} --bin` works normally
6. 🐞 `op run op` should work

## REGRESSION TEST : 

From the seed you can use alternative paths (tmp it is better)
1. 📣 `op run gabriel-greeting-dart --listen stdio://` should be rejected.
2. 📣 `op run gabriel-greeting-dart` should use tcp.
3. 📣 `op build gabriel-greeting-dart`, `mkdir -p ~/Desktop/isolated && cp examples/hello-world/gabriel-greeting-dart/.op/build/gabriel-greeting-dart.holon  ~/Desktop/isolated` `cd ~/Desktop/ && op list --root ~/Desktop/templates` should not be able to find the dart holon `~/Desktop/isolated` AND  `op list` should be able to find it
4. 📣 `op run op` should work as any holon.
5. BENCHMARK : op list --root ~/ should remain fast ( current response on my mac os == `op list --root ~/  2.06s user 9.55s system 41% cpu 28.253 total`)

⚠️ GENERAL QUESTION how can we automate integration test using op and helloworld samples
WHAT IS THE NAME FOR SUCH TESTS ? 

## Run flags

| Flag | Value | Default | Description |
|---|---|---|---|
| `--listen` | `<uri>` | `tcp://127.0.0.1:0` | Transport listen URI |
| `--no-build` | — | `false` | Skip auto-build before running |
| `--target` | `<value>` | `""` | Build target |
| `--mode` | `<value>` | `""` | Build mode |

## Global flags

| Flag | Value | Description |
|---|---|---|
| `--format` | `json` | Output in JSON format |
| `--quiet` / `-q` | — | Suppress progress output |
| `--root` | `<path>` | Override discovery root |
| `--bin` | — | Print resolved binary path and exit |




### --format 
### --quiet
### --root 
### --bin

## Default behaviour `op run gabriel-greeting-go`
`op run gabriel-greeting-go` == `op run gabriel-greeting-go --listen tcp://120.0.0.1:0`


## `op run gabriel-greeting-go` != `op gabriel-greeting-go`
`op gabriel-greeting-go` requires a second arg (a method name). Without one it errors: missing command for holon "gabriel-greeting-go"


## tcp 

```shell
 op run gabriel-greeting-go --listen tcp://127.0.0.1:60001
 ```

---

# How `op run` works

1. **Global flags parsed** in [parseGlobalOptions](./api/cli.go#L205-L256) — extracts `--format`, `--quiet`, `--root`, `--bin` before the subcommand.
2. **Dispatch** — `"run"` case in [run()](./api/cli.go#L104-L105) calls `runRunCommand`.
3. **Flag parsing** — [parseRunArgs](./api/cli_lifecycle.go#L208-L243) extracts `--listen` (default `stdio://`), `--no-build`, `--target`, `--mode`.
4. **Request resolution** — [resolveRunRequest](./api/run_helpers.go#L296-L308) builds the `RunRequest` protobuf.
5. **Execution** — [runWithIO](./api/run_helpers.go#L34-L157) does the heavy lifting:
   - Resolves holon target via [ResolveTarget](./api/run_helpers.go#L84)
   - Checks installed binary via [ResolveInstalledBinary](./api/run_helpers.go#L88)
   - Auto-builds if artifact missing (unless `--no-build`)
   - Constructs the final command via [commandForArtifact](./api/run_helpers.go#L189-L225) → `<binary> serve --listen <uri>`
   - Runs in foreground with signal forwarding via [runForeground](./api/run_helpers.go#L243-L271)

----

# Useful commands: 

## Reinstall a fresh op : 

```shell
op install op --build
```

## Kill a holon bin: 

```shell
pkill -9 -f gabriel-greeting-go
```

## tcp: 
```shell
# Find the PID
lsof -ti tcp:60001

# Kill it
kill $(lsof -ti tcp:60001)

# Or force-kill if needed
kill -9 $(lsof -ti tcp:60001)
```



