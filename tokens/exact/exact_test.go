package exact

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
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

// naiveTokens is the quadratic reference merge loop.
func (b *BPE) naiveTokens(piece string) int {
	if _, ok := b.ranks[piece]; ok {
		return 1
	}
	parts := make([]int, len(piece)+1)
	for i := range parts {
		parts[i] = i
	}
	for len(parts) > 2 {
		best, at := -1, -1
		for i := 0; i+2 < len(parts); i++ {
			if r, ok := b.ranks[piece[parts[i]:parts[i+2]]]; ok && (at < 0 || r < best) {
				best, at = r, i
			}
		}
		if at < 0 {
			break
		}
		parts = append(parts[:at+1], parts[at+2:]...)
	}
	return len(parts) - 1
}

func testBPE(t *testing.T, toks ...string) *BPE {
	t.Helper()
	var vocab strings.Builder
	for i, tok := range toks {
		fmt.Fprintf(&vocab, "%s %d\n", base64.StdEncoding.EncodeToString([]byte(tok)), i)
	}
	b, err := parseVocab([]byte(vocab.String()))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPieceTokensMatchesNaive(t *testing.T) {
	b := testBPE(t, "a", "b", "c", "ab", "bc", "abc", "ca", "aab", "cab", "bca", "abca")
	for n := range 400 {
		var s strings.Builder
		x := uint32(n*2654435761 + 1)
		for range n % 40 {
			x = x*1664525 + 1013904223
			s.WriteByte("abcd"[x>>24%4])
		}
		if got, want := b.pieceTokens(s.String()), b.naiveTokens(s.String()); got != want {
			t.Errorf("pieceTokens(%q) = %d, want %d", s.String(), got, want)
		}
	}
}

// A long piece with no spaces (minified or base64 data) must not take
// quadratic time.
func TestPieceTokensLong(t *testing.T) {
	b := testBPE(t, "a", "b", "ab")
	piece := strings.Repeat("ab", 200_000)
	done := make(chan int)
	go func() { done <- b.pieceTokens(piece) }()
	select {
	case got := <-done:
		if got != 200_000 {
			t.Errorf("pieceTokens = %d, want 200000", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pieceTokens still running after 5s")
	}
}
