status: todo

`trace` handles JVM, Python, Rust, Ruby, .NET, PHP, gdb/sanitizers, D, but not Node
(`architecture/families.md:83` leaves Node frames to family engines). A plain `node app.js` or
`npm start` crash lands in `doc/text` with every frame kept.

Scope: add a row to `internal/compress/trace/frames.go`: frame regex `at fn (file:line:col)`, noise
`node:internal|node_modules/`, keep the top user frame; cover Deno and Bun shapes. Real crash fixtures.

Done when: `make check` and `make eval` pass, `raw ok` never `NO`; `.harness/architecture/families.md`
trace section updated in the same commit.
