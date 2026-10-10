package npmoptimizer

import (
	"testing"

	"github.com/lukaszb/gascoigne/compress"
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

func TestTSC(t *testing.T) {
	in := compress.Lines("src/a.ts(3,3): error TS2322: Type 'string' is not assignable.\n  Type 'x' is not 'y'.\nsrc/a.ts(9,1): error TS7006: any\nsrc/b.ts(1,1): error TS1005: ';' expected.\n")
	want := "src/a.ts:\n  (3,3) TS2322: Type 'string' is not assignable.\n    Type 'x' is not 'y'.\n  (9,1) TS7006: any\nsrc/b.ts:\n  (1,1) TS1005: ';' expected.\n"
	if got := compress.Join(TSC(in)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
