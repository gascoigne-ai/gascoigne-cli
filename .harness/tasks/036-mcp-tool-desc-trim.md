status: todo
# feat(mcp-proxy): trim tool descriptions

Backlog [16]: MCP tool schemas cost about 35k tokens per request. The `mcp-proxy` could cap tool
descriptions with `compress/terse`.

Scope: trim only descriptions, never schemas or parameter names. Needs an A/B (`testdata/ab/`) to show
tool-choice accuracy does not drop. Check cache impact: descriptions are in the prefix, so trimming must
be deterministic and stable across requests.

Done when: A/B result in backlog; `make check` and `make eval-mcp` pass; architecture doc updated in the
same commit.
