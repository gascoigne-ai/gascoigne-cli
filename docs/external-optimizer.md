# External optimizers

Run your own program as an optimizer: a script, a binary, any tool that reads a block of text and
prints a shorter one. No Go, no fork. Working examples are in
[`examples/external-optimizers/`](../examples/external-optimizers/).

An external optimizer is the built-in `external` type configured under a name, `external/<name>`.
Everything every optimizer gets applies: the original stays retrievable (`[raw:<id>]`), a failure
returns the original output, and a rewrite that saves too little is dropped.

## Configure

In **your** config (`$XDG_CONFIG_HOME/gascoigne/config.yaml`, `-config`, or `-set`). A repository's
`.gascoigne.yaml` is refused: it would let a repo choose which commands run on your machine.

```yaml
optimizers:
  external/levels:
    command: [python3, /abs/path/log-levels.py]   # argv, no shell; use absolute paths
    protocol: json          # text | json | rpc
    types: [log/*]          # block types (trailing /* = family); empty = every block
    cmds: ["tail *"]        # globs on the command line; empty = any. Types AND cmds must match
    timeout: 5s             # one call (rpc: one request)
    max_input_tokens: 0     # >0: bigger blocks are not sent
    max_output_bytes: 8388608
    dir: ""                 # working directory, "" = caller's
    env: { LEVELS: "warn" } # extra variables
    pass_env: []            # variables forwarded from gascoigne's own environment

router:
  default: [external/levels, embedjson, trace, ...]   # where it runs in the chain
pipelines:
  exec:
    optimizers: [external/levels, embedjson, trace, ...]   # and in each pipeline that should use it
```

Lists replace the defaults, so copy the current `router.default` and pipeline `optimizers` from
`gascoigne config` and add your id. An optimizer the pipeline does not list never runs there. An
external optimizer sees what the ones before it left, so order matters.

The child gets a scrubbed environment: only `PATH`, `LANG`, `LC_ALL`, `TMPDIR`, `TERM`, `pass_env`
and `env`. API keys and tokens in your shell do not reach it.

## Contract (version 1)

Your program must be **deterministic** (same input, same output: prompt caches and evals depend on
it), **idempotent** and **offline**. Drop noise, never facts the agent needs; what you drop stays
retrievable, but every retrieval costs the agent a step.

The child receives one block at a time (each matching block of the payload).

### `text`

| | |
|---|---|
| stdin | the block text |
| stdout | the new text |
| exit | `0`; anything else is a failure |
| env | `GASCOIGNE_PROTOCOL=1`, `GASCOIGNE_BLOCK_TYPE`, `GASCOIGNE_BLOCK_KIND`, `GASCOIGNE_ATTR_<KEY>` for every payload attribute (`cmd`, `file`, `query`, ...; keys upper-cased, other characters `_`) |

The simplest contract: any filter works, `sed`, `jq`, `awk`. Output equal to input means "nothing to do".

### `json`

One JSON document on stdin, one on stdout, exit `0`.

```json
{"version":1,"block":{"kind":"cmd_output","type":"log/app","text":"..."},"attrs":{"cmd":"tail app.log"}}
```
```json
{"version":1,"text":"shorter text","skip_reason":"optional, why nothing changed"}
```

`{"version":1,"error":"message"}` reports a failure the child understands. `text` is required.
`skip_reason` shows up in `gascoigne gain` skip statistics when nothing was shortened.

### `rpc`

A long-lived child for slow starts. Messages are the `json` ones, one per line (newline-delimited,
no embedded newlines), plus an `id` that the reply must repeat.

1. gascoigne starts the child on first use and sends `{"hello":1}`; reply `{"version":1}`.
2. For each block: request `{"version":1,"id":7,"block":{...},"attrs":{...}}`, reply
   `{"version":1,"id":7,"text":"..."}` (or `"error"`). One request at a time.
3. Flush stdout after every reply. Exit when stdin closes.

A crashed child is restarted once per call. A request that outlives `timeout` kills the child, and
the next call starts a fresh one. Write logs to stderr, never stdout.

## What gascoigne enforces

A call fails, and the original output is used, on: timeout, non-zero exit, unparsable or
wrong-version reply, missing or empty `text` for non-empty input, output over `max_output_bytes`.
The failure is recorded like any optimizer failure. Stderr (last 2 KB) is logged at debug level.
A reply that is not smaller (estimated tokens) is ignored, so a child cannot make output bigger.
On Unix the child runs in its own process group and a timeout kills all of it.

The schema of the messages is in [`external-optimizer.schema.json`](external-optimizer.schema.json).
