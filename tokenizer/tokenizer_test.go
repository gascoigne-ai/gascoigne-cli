package tokenizer

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Splits as the reference implementation of o200k_base makes them.
func TestSplit(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"Hello, world!", []string{"Hello", ",", " world", "!"}},
		{"  foo  bar\n\n  baz", []string{" ", " foo", " ", " bar", "\n\n", " ", " baz"}},
		{"x = 12345;\t\ty", []string{"x", " =", " ", "123", "45", ";", "\t", "\ty"}},
		{"It's DON'T stop", []string{"It's", " DON'T", " stop"}},
		{"go1.27 ✓ 日本語テキスト", []string{"go", "1", ".", "27", " ✓", " 日本語テキスト"}},
		{" \n  ", []string{" \n", "  "}},
	}
	for _, tt := range tests {
		if got := split(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("split(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBPEMerges(t *testing.T) {
	var vocab strings.Builder
	for i, tok := range []string{"a", "b", "c", "ab", "bc", "abc", " "} {
		fmt.Fprintf(&vocab, "%s %d\n", base64.StdEncoding.EncodeToString([]byte(tok)), i)
	}
	b, err := parseVocab([]byte(vocab.String()))
	if err != nil {
		t.Fatal(err)
	}
	// "abcab": whole piece unknown; ab (rank 3) merges before bc (4):
	// ab|c|ab, then abc (5) → abc|ab.
	for in, want := range map[string]int{"abc": 1, "abcab": 2, "cba": 3, "ab ab": 3} {
		if got := b.Count(in); got != want {
			t.Errorf("Count(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestClaudeCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" || r.Header.Get("x-api-key") != "k" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		var body strings.Builder
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body.Write(buf[:n])
		// Frame of 7 tokens; "." is one token, the text 40.
		if strings.Contains(body.String(), `"content":"."`) {
			fmt.Fprint(w, `{"input_tokens":8}`)
			return
		}
		fmt.Fprint(w, `{"input_tokens":47}`)
	}))
	defer srv.Close()
	n, err := Claude{Key: "k", Model: "claude-opus-5-5", URL: srv.URL}.Count(t.Context(), "some text")
	if err != nil || n != 40 {
		t.Fatalf("got %d, %v; want 40", n, err)
	}
	if _, err := (Claude{Key: "x", URL: srv.URL}).Count(t.Context(), "t"); err == nil {
		t.Fatal("bad key: want error")
	}
	if _, err := (Claude{}).Count(t.Context(), "t"); err != ErrNoKey {
		t.Fatalf("no key: %v", err)
	}
}
