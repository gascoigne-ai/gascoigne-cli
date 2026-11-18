status: todo
kind: chore
commit: refactor(optimizer): split long functions, flatten nesting
scope: optimizer/**

# Split long functions in optimizers

~24 functions over 40 lines, worst: `logoptimizer/log.go:158` `template` (74), `dataoptimizer/toml.go:28` (72), `infraoptimizer/others.go:47` `ansible` (63), `docoptimizer/doc.go:196` `helpFor` (59), `traceoptimizer/trace.go:61` (56), `logoptimizer/access.go:27` (54). Nesting 4: `logoptimizer/log.go:67`, `entries.go:102`, `docoptimizer/doc.go:80,231`, `dataoptimizer/xml.go:56`, `catoptimizer/outline.go:86,379`, `ariaoptimizer/aria.go:128`. Split parse from render.

Output byte-identical. Acceptance: `make check`, `make eval`.
