package npmoptimizer

import (
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(typ, in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}}
	Optimizer{}.Optimize(nil, p)
	return p.Blocks[0].Text
}

func TestInstall(t *testing.T) {
	in := `npm warn deprecated a@1.0.0: old
npm http fetch GET 200 https://registry.npmjs.org/b 12ms
npm warn deprecated b@2.0.0: older

added 10 packages, and audited 11 packages in 1s

3 packages are looking for funding
  run ` + "`npm fund`" + ` for details

2 vulnerabilities (1 moderate, 1 high)

To address all issues, run:
  npm audit fix
`
	want := "npm warn deprecated (2): a@1.0.0, b@2.0.0\n\nadded 10 packages, and audited 11 packages in 1s\n\n2 vulnerabilities (1 moderate, 1 high)\n\n"
	if got := run(detect.NPM, in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
