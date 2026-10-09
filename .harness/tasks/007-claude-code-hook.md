status: todo

# Claude Code hook: route Bash through gascoigne exec

## Goal
Agents get optimized command output without changing how they call tools.

## Scope
- `gascoigne hook` subcommand implementing a Claude Code PreToolUse hook for `Bash`: reads the
  hook JSON on stdin, rewrites `tool_input.command` to `gascoigne exec -- sh -c '<command>'`
  when the command's first word maps to a known content type (`detect.FromCmd`), else leaves it.
- Never rewrite commands already wrapped, with `-raw`, or containing pipes/redirects into files
  (the agent post-processes those itself).
- `gascoigne hook install [--project]` prints/merges the settings.json snippet.

## Acceptance
- Table tests: rewrite / no-rewrite cases, quoting of single quotes, idempotency.
- Manual: in a scratch repo, `go test ./...` via Claude Code shows optimized output with hint.
- `make check` green.
