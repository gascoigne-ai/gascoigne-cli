status: todo

`cat.clean_min_tokens` is 800 (`configs/default.yaml:66`). About 1.5M tokens a month of 200-800 token
`sed`/`cat` source reads stay whole. Comment stripping saves 14-50% of source tokens (recommendations.md),
so roughly 150-300k tokens a month. Risk: `sed -n a,bp` ranges are edit targets, and an Edit `old_string`
spanning a `// … comments L12-30` gap fails (backlog measured this on Read).

Scope: try 400 for whole-file `cat` reads only, not `sed -n` slices (`detect.ReadsFile`). Validate with
`ab.sh` on edit tasks before changing the default. Read backlog "Tried and dropped" first.

Done when: A/B result recorded in backlog; default changed only if edit tasks do not regress;
`make check` and `make eval` pass.
