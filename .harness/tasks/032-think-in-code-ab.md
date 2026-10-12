status: todo

# A/B: "write a script, print only the result" guide line

## Goal
Agents often read dozens of files to count or aggregate. One guide sentence ("to count or
aggregate across many files, write a short script and print only the result") may cut input tokens.
Measure before shipping; no option or flag for it.

## Scope
- Add 2-3 tasks to `testdata/ab` that need aggregation over many files (with `setup.patch`, `check.sh`).
- Run `.harness/scripts/ab.sh` with `AB_TAG` for the build with and without the line (needs an
  API key; `-n` first). Repeats >= 3.
- Keep the line only if success stays 100% and cost per run drops; record both results in
  `recommendations.md` Measured (as the brevity-line entry does); otherwise record under Tried and dropped.

## Acceptance
- `ab.sh -n` passes for the new tasks; the result, kept or dropped, is in `recommendations.md`.
