status: todo
# feat(config): project config overlay

`configPath` (`cmd/gascoigne/userconfig.go:109-118`) knows only `-config`, `$GASCOIGNE_CONFIG` and XDG.
A repo cannot set its own `truncate.max_tokens`, `router.default`, `mcp_proxy.rules` or `-disable`.

Scope: find `.gascoigne.yaml` from cwd up to the git root in `exec` and `hook` and lay it over the user
file. Keep strict decoding. The server keeps one warm pipeline per config hash (`server.md`), so the
overlay must feed the hash.

Done when: tests for lookup order, overlay merge and bad keys; `make check` passes;
`.harness/architecture/` config doc updated in the same commit.
