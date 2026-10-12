status: later

# Resume snapshot after compaction (experimental)

## Goal
After the agent compacts its conversation it forgets which files it edited and what it was doing.
Inject a short, capped snapshot built from the ledger and transcript. Opt-in until it pays off.

## Scope
- Claude `PreCompact` and SessionStart(source compact/resume) hooks; gemini `PreCompress`.
  Snapshot: files edited, last user prompt, open tasks, last failed commands, about 500 tokens
  at most, deterministic.
- `experimental: [continuity]`; library under `compress/` or `cmd/gascoigne`, no core change.

## Acceptance
- Fixture transcript test: size cap, determinism, no secrets from tool output.
- A/B on a compaction-forcing task: tokens added vs rework saved, recorded in `recommendations.md`.
Blocked until tasks 033-035 land (hook registration and manifest).
