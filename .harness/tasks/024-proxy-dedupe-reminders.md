status: todo

# Proxy: fold repeated reminders

## Goal
Per-prompt hook contexts and `<system-reminder>`s repeat byte-identical across user turns.

## Scope
- Opt-in config. A later identical reminder in one request → `[reminder: same as earlier]`;
  depends only on earlier messages (prefix-stable like `fold.go`), original via `SaveRaw`.
- Measure with task 023 counts first; drop if < 1% of instruction tokens.

## Acceptance
- Prefix stability test (request N's forwarded prefix equals request N+1's); `make check` green.
