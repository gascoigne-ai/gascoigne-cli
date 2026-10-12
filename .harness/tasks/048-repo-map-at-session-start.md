status: todo

# Repo map at SessionStart

## Goal
Sessions begin with `ls`, `find` and `grep` turns to learn the layout. Each of those turns re-reads
the whole context. A small deterministic map in the session-start context can replace several of
them. The map is re-read on every request too, so it must pay for itself.

## Scope
- Claude SessionStart `additionalContext`, opt-in (experimental `repomap`, a setup question like
  `continuity`): top-level tree, definitions per package from the `show` library, build and test
  command from the Makefile or package manifest. At most 2000 tokens, deterministic per commit
  (no timestamps), cut by directory depth then by size.
- Skipped on compact and resume (the snapshot covers them) and when the guide level is `off`.
- Library under `compress/symbols` reuse; no core change.

## Acceptance
- Golden test on a fixture repo: cap, determinism, ignores `.git`, vendored and generated dirs.
- `ab.sh` with and without the map: turns and cost per task, success stays 100%. Keep it
  experimental until the A/B shows fewer turns than the 2k tokens cost. Record in `backlog.md`.
