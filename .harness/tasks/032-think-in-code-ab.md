status: later

# A/B: "write a script, print only the result" guide line

## Goal
Agents often read dozens of files to count or aggregate. One guide sentence ("to count or
aggregate across many files, write a short script and print only the result") may cut input tokens.
Measure before shipping; no option or flag for it.

## Done
Tasks `count-optimizer-tests` and `count-lazyre-files` in `testdata/ab` need aggregation over many
files; `ab.sh -n` passes for both.

## Scope (needs an API key and money, so run by hand)
- Build A (guide as is) and build B (the sentence added to `guide()` in `guide.go`); run
  `AB_TAG=base .harness/scripts/ab.sh -a on` and `AB_TAG=line .harness/scripts/ab.sh -a on`
  (OpenRouter: `AB_ENV_FILE`), repeats >= 3, all tasks.
- Keep the line only if success stays 100% and cost per run drops; record both results in
  `recommendations.md` Measured (as the brevity-line entry does); otherwise record under Tried and dropped.

## Acceptance
- The result, kept or dropped, is in `recommendations.md`; if kept, `guide.go` and its test have the line.
