package main

import (
	"context"
	"os"
	"testing"
)

// With no file and a terminal on stdin, tokens must fail at once instead
// of waiting for input that never ends. A character device stands in for
// the terminal.
func TestTokensTerminalStdin(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	old := os.Stdin
	os.Stdin = null
	defer func() { os.Stdin = old }()
	if err := cmdTokens(context.Background(), nil); err == nil {
		t.Error("cmdTokens with terminal stdin and no file returned nil, want error")
	}
}
