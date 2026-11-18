status: todo
kind: feature
commit: feat(hook): wrap compound shell lines per segment
scope: cmd/gascoigne/hook.go, cmd/gascoigne/hook_parity_test.go, .harness/architecture/hooks.md

# Per-segment rewrite of compound Bash lines (highest-ROI item)

`wrapCommand` returns the whole line unwrapped when it contains a newline, a pipe other than head/tail
in a non-last segment, a redirect, or `$(`. Such lines reach PostToolUse as one blob typed from the first
work command. In the live ledger the hook path is about 3.3M raw tokens at 11-16% saved, against about
50% for wrapped (`exec`) commands. Pipe lines 1.56M to 1.39M, lists 550k to 495k, heredocs 482k to 405k.

Change:
- Leave an unwrappable segment verbatim and keep wrapping the others.
- Allow `wrapPipeline` mid-list, inside `{ ...; }` so `PIPESTATUS` and the exit status stay correct.
- Keep bailing on loops, subshells and heredoc bodies.
- Bump `registrationVersion`.

Reuse `segments`, `splitStages`, `hasShellOperator`, `TestWrapPipelineBashParity`, `TestWrapCommand`.

Test first: extend the parity set (306 commands) with mixed lines; assert byte-identical stdout and
identical exit status between original and rewritten commands in a bash sandbox.

Risk: shell semantics (`&&` chains, `set -e`, exit status of the last segment). Segments under the
200-token floor each pay a hint, so measure the net effect and do not wrap segments that cannot pay.

Acceptance: parity tests pass; a replay of recent hook-path commands shows hook share of raw falling
and exec share rising; `rerequested` count does not rise. Expected net 150-250k tokens per 7 weeks; drop
the change if a replay shows under 50k. `make check && make eval` pass.

Model: unchanged
