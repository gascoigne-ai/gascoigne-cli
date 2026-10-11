# Getting started

## Install

```sh
go install github.com/lukaszb/gascoigne/cmd/gascoigne@latest
```

Or from a checkout:

```sh
make install                          # → ~/.local/bin/gascoigne
make install PREFIX=/opt/gascoigne    # → /opt/gascoigne/bin/gascoigne
make uninstall                        # same PREFIX as install
make build                            # → bin/gascoigne, for development
```

For every user: `make build && sudo install -m755 bin/gascoigne /usr/local/bin/` (root's
`PATH` often lacks `go`). Install it where `PATH` finds it: rewritten agent commands then call
plain `gascoigne` instead of a full path. Reinstalling replaces a running background server
from the old build on its next use.

gascoigne is pure Go (`CGO_ENABLED=0`), including its sqlite driver, so the binary has no
system dependencies.

## Run a command through it

```sh
gascoigne exec -- go test ./...
```

`exec` runs the command, merges stdout and stderr, optimizes the result and prints it with a
hint line. The exit code is the command's own.

```
[gascoigne: optimized 1488→132 tokens · full: gascoigne raw 9f1c2e7a]
```

The hint appears only when the output changed. Small outputs (under 200 tokens) and output
gascoigne has no engine for pass through byte-exact, with no hint.

## Optimize anything on stdin

```sh
kubectl get pods -A -o yaml | gascoigne run
cat access.log | gascoigne run -attr file=access.log
```

On stdin there is no command to detect the content from, so gascoigne sniffs the text. Hints
help: `-attr cmd='terraform plan'` or `-attr file=deploy.yaml` make detection exact.

## Get the original back

```sh
gascoigne raw 9f1c2e7a
```

The original is printed byte-exact. Fetching it also tells gascoigne the optimization was not
good enough for that source (the same command, file or URL), so next time that source passes
through untouched. See [Ledger, retrieval and gain](ledger.md) to undo that.

Often one detail is enough, and reading only it does not mark the source:

```sh
gascoigne raw 9f1c2e7a -grep 'FAIL|panic' -C 2      # matching lines with 2 around, numbered
gascoigne raw 9f1c2e7a -query "connection refused"  # the lines that best match the words
gascoigne raw 9f1c2e7a -lines 200-240               # a line range
```

## Read one function

```sh
gascoigne show internal/service/pipeline.go          # definitions with line ranges
gascoigne show internal/service/pipeline.go:Process  # one, with its doc comment and line numbers
```

Agents get the same as the MCP `show` tool, and the usage guide tells them to prefer it over
reading a large file whole.

To skip optimization up front:

```sh
gascoigne exec -raw -- git diff
GASCOIGNE_RAW=1 gascoigne exec -- git diff
```

## See the savings

```sh
gascoigne gain
gascoigne gain -by command
gascoigne gain -since 7d -by day
```

Or watch it live in the browser at http://127.0.0.1:7878. The first gascoigne command
started a small background server for it; `gascoigne dashboard` prints the address and
starts it if needed, `gascoigne server stop` stops it. It exits by itself after 30 minutes
without a request and the next command starts it again.

Every run appears as it happens, with its savings, content type and, when it was not
optimized, the reason. Click a run to compare the original with what the agent read, side by
side, and see each optimizer's step.

## Use it with an agent

```sh
gascoigne wrap claude      # or: codex, gemini, opencode, pi
```

This starts the agent with gascoigne registered for that run only: shell commands
go through `gascoigne exec`, other MCP servers' results are optimized, the agent's API traffic
goes through the LLM proxy in the background server (Claude's Read is shortened too, line
numbers kept), and the MCP server adds `fetch`, `compress`, `show`, `retrieve` and `stats`. No config
file is touched. `gascoigne install` registers it for good. See [Agents](agents.md).

## Where things live

| What | Where |
|---|---|
| ledger (originals, events, failures) | `$XDG_DATA_HOME/gascoigne/ledger.db` (usually `~/.local/share/gascoigne/ledger.db`) |
| your config | `$XDG_CONFIG_HOME/gascoigne/config.yaml`, written on first run with every setting commented out (or `$GASCOIGNE_CONFIG`, `-config`) |
| built-in config | `configs/default.yaml`, embedded in the binary |

Originals are kept for 24 hours by default (`extensions.ledger.raw_ttl`). Events and savings
are kept until you clear them with `gascoigne reset`.

| background server log | `$XDG_CACHE_HOME/gascoigne/server.log` (usually `~/.cache/gascoigne/server.log`) |

## Next

- [Families and dialects](families.md): what each kind of output turns into.
- [Configuration](configuration.md): change limits without a config file (`-set`).
