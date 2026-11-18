package execreceiver

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

func TestFactoryAppliesOptions(t *testing.T) {
	next := &capture{}
	inv := &Invocation{Args: []string{"echo", "hi"}, Stdin: strings.NewReader("")}
	f := NewFactory(WithInvocation(inv))
	c, err := f.Create(t.Context(), component.Settings{Pipeline: next}, f.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.(*Receiver).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if next.got == nil || next.got.Text() != "hi\n" {
		t.Fatalf("pipeline got %v, want the output of the invocation", next.got)
	}
}
