// Package exact counts tokens exactly, where tokens.Estimate only
// approximates: o200k_base (the GPT-4o family's byte-pair encoding)
// reimplemented in Go over its published vocabulary, and Claude through
// Anthropic's token counting endpoint. Both cost a download or a request,
// so they serve measurements (gascoigne tokens), not the optimizer path.
package exact

import (
	"bufio"
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

const (
	o200kURL = "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"
	// o200kSHA256 is the vocabulary's published hash: a download that does
	// not match is refused.
	o200kSHA256 = "446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"
	o200kFile   = "o200k_base.tiktoken"
)

// o200kSplit is the encoding's pre-tokenizer without its one lookahead,
// \s+(?!\S), which split emulates: words with their leading punctuation or
// space, numbers in runs of up to three digits, punctuation runs, newlines.
var o200kSplit = lazyre.New(`[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?` +
	`|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]+[\p{Ll}\p{Lm}\p{Lo}\p{M}]*(?i:'s|'t|'re|'ve|'m|'ll|'d)?` +
	`|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n/]*|\s*[\r\n]+|\s+`)

// httpClient bounds every request: a stalled network must end in an error,
// not a hung command.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// BPE is a byte-pair encoding: token bytes → merge rank.
type BPE struct {
	ranks map[string]int
}

// O200k loads the o200k_base vocabulary from dir, downloading it there
// first (3.6 MB, hash checked) when missing.
func O200k(ctx context.Context, dir string) (*BPE, error) {
	path := filepath.Join(dir, o200kFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if data, err = download(ctx, o200kURL); err != nil {
			return nil, fmt.Errorf("o200k vocabulary: %w", err)
		}
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != o200kSHA256 {
		os.Remove(path)
		return nil, fmt.Errorf("o200k vocabulary: hash mismatch, removed %s", path)
	}
	if err := os.MkdirAll(dir, 0o755); err == nil {
		os.WriteFile(path, data, 0o644) // a failed cache write only costs a later download
	}
	return parseVocab(data)
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// parseVocab reads "base64token rank" lines.
func parseVocab(data []byte) (*BPE, error) {
	b := &BPE{ranks: make(map[string]int, 200_000)}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		tok, rank, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(tok)
		if err != nil {
			return nil, fmt.Errorf("vocabulary token %q: %w", tok, err)
		}
		r, err := strconv.Atoi(rank)
		if err != nil {
			return nil, fmt.Errorf("vocabulary rank %q: %w", rank, err)
		}
		b.ranks[string(raw)] = r
	}
	return b, sc.Err()
}

// Count is the number of tokens text encodes to (special tokens are
// plain text here, as in tool output).
func (b *BPE) Count(text string) int {
	n := 0
	for _, piece := range split(text) {
		n += b.pieceTokens(piece)
	}
	return n
}

// split pre-tokenizes text. A whitespace run followed by a non-space
// leaves its last character to the next piece (the lookahead \s+(?!\S)).
func split(text string) []string {
	var out []string
	for len(text) > 0 {
		loc := o200kSplit.FindStringIndex(text)
		if loc == nil || loc[0] != 0 {
			// Cannot happen: the alternatives cover every character.
			_, size := utf8.DecodeRuneInString(text)
			loc = []int{0, size}
		}
		end := loc[1]
		m := text[:end]
		if isSpace(m) && end < len(text) && !strings.ContainsAny(m[len(m)-1:], "\r\n") {
			next, _ := utf8.DecodeRuneInString(text[end:])
			if !unicode.IsSpace(next) && utf8.RuneCountInString(m) > 1 {
				_, last := utf8.DecodeLastRuneInString(m)
				end -= last
			}
		}
		out = append(out, text[:end])
		text = text[end:]
	}
	return out
}

func isSpace(s string) bool {
	return strings.TrimFunc(s, unicode.IsSpace) == ""
}

// pair is a merge candidate: the tokens starting at i and j, spanning
// piece[i:end]. It is stale once either token changed.
type pair struct{ rank, i, j, end int }

type pairHeap []pair

func (h pairHeap) Len() int { return len(h) }
func (h pairHeap) Less(a, b int) bool {
	if h[a].rank != h[b].rank {
		return h[a].rank < h[b].rank
	}
	return h[a].i < h[b].i
}
func (h pairHeap) Swap(a, b int) { h[a], h[b] = h[b], h[a] }
func (h *pairHeap) Push(x any)   { *h = append(*h, x.(pair)) }
func (h *pairHeap) Pop() any {
	old := *h
	p := old[len(old)-1]
	*h = old[:len(old)-1]
	return p
}

// pieceTokens merges the piece's bytes, lowest rank first (leftmost on
// ties), until no pair is in the vocabulary. Tokens are a linked list
// keyed by start offset and candidates a heap, so it is O(n log n): a
// long piece without spaces (minified or base64 data) stays fast.
func (b *BPE) pieceTokens(piece string) int {
	if _, ok := b.ranks[piece]; ok {
		return 1
	}
	n := len(piece)
	next := make([]int, n+1)
	prev := make([]int, n+1)
	for i := range next {
		next[i], prev[i] = i+1, i-1
	}
	var h pairHeap
	push := func(i int) {
		if i < 0 {
			return
		}
		j := next[i]
		if j >= n {
			return
		}
		end := next[j]
		if r, ok := b.ranks[piece[i:end]]; ok {
			heap.Push(&h, pair{r, i, j, end})
		}
	}
	for i := range n - 1 {
		push(i)
	}
	count := n
	for len(h) > 0 {
		p := heap.Pop(&h).(pair)
		if next[p.i] != p.j || next[p.j] != p.end {
			continue // stale
		}
		next[p.i] = p.end
		prev[p.end] = p.i
		next[p.j] = -1
		count--
		push(prev[p.i])
		push(p.i)
	}
	return count
}
