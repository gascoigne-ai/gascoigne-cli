package bpe

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// Reference ids come from transformers' tokenizer for ModernBERT on pre-split
// words (testdata/ref.json, generated once). PROSE_TOKENIZER points at the
// model's tokenizer.json; without it the test is skipped.
func TestWordsMatchReference(t *testing.T) {
	for _, c := range []struct{ env, ref string }{
		{"PROSE_TOKENIZER", "testdata/ref.json"},
		{"PROSE_TOKENIZER_AGENT", "testdata/ref-agent.json"},
	} {
		t.Run(c.env, func(t *testing.T) { matchReference(t, os.Getenv(c.env), c.ref) })
	}
}

func matchReference(t *testing.T, path, ref string) {
	if path == "" {
		t.Skip("set the env var to the model's tokenizer.json")
	}
	tok, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ref)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Words []string
		IDs   []int `json:"ids"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got := []int{tok.CLS}
		for _, w := range c.Words {
			got = append(got, tok.Word(w)...)
		}
		got = append(got, tok.SEP)
		if !slices.Equal(got, c.IDs) {
			t.Errorf("%q:\n got %v\nwant %v", c.Words, got, c.IDs)
		}
	}
}
