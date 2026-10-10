package main

import (
	"testing"

	"github.com/lukaszb/gascoigne/lazyre"
)

// Patterns compile on first use; this binary links every component, so a
// bad one fails here rather than in the middle of an agent's run.
func TestPatternsCompile(t *testing.T) {
	if err := lazyre.Check(); err != nil {
		t.Fatal(err)
	}
}
