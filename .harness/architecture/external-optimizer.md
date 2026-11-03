# External optimizer

`optimizer/externaloptimizer` (type `external`, family `external`) runs a user's command as an
optimizer. User contract: `docs/external-optimizer.md`; wire schema `docs/external-optimizer.schema.json`;
examples `examples/external-optimizers/`.

## Shape
- Instances are ids `external/<name>`; one process config per id (`Config`: command argv, protocol,
  `types`/`cmds` selectors, timeout, size limits, env). No default instance exists, so the config
  template documents it with `##` lines only.
- `Optimize` walks the blocks, skips those `Config.wants(type, cmd)` rejects (type or `family/*`
  match AND glob on the `cmd` attr), and asks the codec per block.
- Protocols are a table, `codecs` in `codec.go`: `text` and `json` start the child per block
  (`codec_oneshot.go`, one `oneshot` type with encode/decode funcs), `rpc` keeps one child
  (`codec_rpc.go`: lazy start, `hello` handshake, id-matched lines, one restart on a dead child, timeout kills the child).
- `proc.go`: argv without a shell, environment scrubbed to `baseEnv` + `pass_env` + `env`, stdout capped
  at `max_output_bytes`, stderr tail kept for errors/debug log. `proc_unix.go` runs the child in its own
  process group and kills the group on cancel.

## Guarantees (all in this package; core is unchanged)
- Any failure returns an error: core then keeps the original payload and records a failure.
- Empty text for non-empty input is an error; a reply not smaller in estimated tokens is ignored
  (`AttrSkip` set, with the child's `skip_reason` when given).
- Determinism is the child's duty (documented contract); core snapshots the original to the ledger.

## Trust
`cmd/gascoigne/userconfig.go` `denyExternal`: `.gascoigne.yaml` may not define `external` optimizers
(a repo must not choose commands to run). User config, `-config`, `$GASCOIGNE_CONFIG` and `-set` may.

## Adding to a chain
Lists replace on merge: the id goes into `router.default` (or a route) and into each pipeline's
`optimizers` list; a router entry a pipeline does not list never runs there (`config.md`).

## Tests
`external_test.go` re-executes the test binary as the child (`TestHelperProcess`, steered by
`EXTOPT_MODE`) for every protocol and failure; `examples_test.go` runs the shipped examples on eval
fixtures (skips when an interpreter is missing).
