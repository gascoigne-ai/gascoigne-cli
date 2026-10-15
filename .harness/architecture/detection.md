# Content detection: `family/dialect` (`internal/detect/`, `filter/detectfilter`)

Every block gets a type `family/dialect` (`test/jest`, `build/tsc`, `pkg/pip`, `task/nx`). One
payload can mix types, and each part meets its own family engine.

## Dialect specs

Dialects are declarative `Spec`s in `internal/detect/<family>.go`:
- command patterns: `program sub sub -flag`, the most specific wins; `CmdFunc` for flag values
- file globs and MIME types
- a sniffer with a priority: composite runners first, generic formats last

Tools whose family engine reads rules from a table (`diag.rules`, `testreport.dialects`, …) are
declared as `detect.Row`s (`rows_<family>.go`): type, command patterns, one sniff pattern, plus
an example command and sample text that `TestRows` checks, so no separate matrix entry is needed.
`build/generic` is the shape fallback: three or more `file:line: error|warning` lines (before
`search/grep`, whose shape it shares) get the diagnostic engine with no tool-specific rules.

`Argv` unwraps wrappers and runners (`sudo`, `npx`, `uv run`, `bundle exec`, `python -m`).

## Evidence order

command → file/MIME → sniffing → `doc/text`.

A generic type from command, file or MIME is refined by a spec that lists it in `Refines` and
whose sniffer matches: `cat deploy.yaml` with manifests → `k8s/manifest`, a saved aws response →
`cloud/aws`.

## Helpers and rules

- `internal/detect/matrix_test.go` requires a command example for every dialect with command patterns,
  and a content example (an eval fixture) for every dialect with a sniffer.
- `detect.Family` / `detect.Dialect` split a type.
- `detect.Command` names commands for stats; leading `cd`/`export` steps of a list are skipped.
- `detect.FromCmd` and `ReadsFile` look at the first stage of a pipeline (`firstStage`, after `cd`-style steps):
  later stages only filter its output, and the last word of `sed -n 1,9p a.html | cut -c1-80` is not a file. HTML
  that is not sniffed from a fetcher's output (curl, wget, http, xh, lynx, w3m, links) is typed `fs/code`, never a
  page: before this, such source read through a pipe was converted to its title (6595 → 6 tokens on a real read).
- `edit_safe` (detect filter): config-sized data/k8s/cloud/infra files read from disk
  (`detect.ReadsFile`: cat/head/tail/sed) are typed `fs/code`, so a config read to be edited
  keeps its syntax.
