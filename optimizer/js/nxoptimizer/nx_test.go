package nxoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
)

func TestRewrite(t *testing.T) {
	in := `
 NX   Running target build for 3 projects:

- a
- b
- c

 ———————————————————————————————————————————————

   ✔  nx run a:build  [existing outputs match the cache, left as is]

> nx run b:build

compiled 120 modules
webpack compiled successfully

> nx run c:build

error TS2304: Cannot find name 'x'.
    at run (node_modules/tsc/lib/tsc.js:1:1)
Warning: command "tsc" exited with non-zero status code

 ———————————————————————————————————————————————

 NX   Ran target build for 3 projects (3s)

   ✖  1/3 targets failed, including the following:

      - nx run c:build
`
	got := compress.Join(rewrite(compress.Lines(in)))
	want := `NX   Running target build for 3 projects:
cached: a:build
succeeded: b:build
> nx run c:build
error TS2304: Cannot find name 'x'.
… 1 dependency stack frames omitted
Warning: command "tsc" exited with non-zero status code
 NX   Ran target build for 3 projects (3s)
   ✖  1/3 targets failed, including the following:
      - nx run c:build
`
	got = strings.Join(strings.FieldsFunc(got, func(r rune) bool { return r == '\n' }), "\n") + "\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
