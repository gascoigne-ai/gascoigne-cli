package dataoptimizer

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// TestDispatch pins which handler each content type reaches (and the skip).
func TestDispatch(t *testing.T) {
	o := testOptimizer()
	query := sample.Terms("")
	text := "a,b\n1,2\n3,4\n"
	want := map[string]func(string) (string, error){
		detect.YAML:  func(text string) (string, error) { return o.yaml(text, query) },
		detect.CSV:   func(text string) (string, error) { return o.csv(text, query), nil },
		detect.XML:   o.xml,
		detect.TOML:  func(text string) (string, error) { return toml(text), nil },
		detect.IPYNB: o.notebook,
		detect.Lock:  func(text string) (string, error) { return lock(text), nil },
	}
	for typ, handler := range want {
		t.Run(typ, func(t *testing.T) {
			p := payload.New(payload.CmdOutput, text)
			p.Blocks[0].Type = typ
			exp, expErr := handler(text)
			err := o.Optimize(context.Background(), p)
			if (err != nil) != (expErr != nil) {
				t.Fatalf("err %v want %v", err, expErr)
			}
			if err == nil && p.Blocks[0].Text != exp {
				t.Errorf("got %q want %q", p.Blocks[0].Text, exp)
			}
		})
	}
	p := payload.New(payload.CmdOutput, text)
	p.Blocks[0].Type = detect.JSON
	if err := o.Optimize(context.Background(), p); err != nil || p.Blocks[0].Text != text {
		t.Errorf("json must stay: %q %v", p.Blocks[0].Text, err)
	}
}
