package hookreceiver

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestFactoryAppliesOptions(t *testing.T) {
	out := &Output{Text: "ok\n", Cmd: "go test ./..."}
	f := NewFactory(WithOutput(out))
	c, err := f.Create(t.Context(), component.Settings{Pipeline: tag{}}, f.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.(*Receiver).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if out.Result == nil || out.Result.Attrs[payload.AttrRawID] != "r1" {
		t.Fatalf("result %+v, want the output of the pipeline", out.Result)
	}
}
