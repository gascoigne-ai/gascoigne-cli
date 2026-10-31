status: todo

Unknown output (`wc -l` lists, `sort | uniq -c`, `stat`, `openssl x509 -text`, `nmap`, `dig`, `strace`)
lands in `doc/text` and gets only exact-duplicate folding (`logoptimizer/log.go:33-34`). Risky: backlog
"Tried and dropped" records that generic multi-line repeat folding before the engines was dropped.

Scope: run the template engine (`logoptimizer/log.go:109`) on `doc/text` only, after all engines, when
there are enough lines and the template-count / line-count ratio is low. Keep first occurrence plus
`(×N)`. Must not touch prose. Read backlog first.

Done when: `adversarial` goldens unchanged; savings only on repetitive cases; `make check` and
`make eval` pass; architecture doc updated in the same commit.
