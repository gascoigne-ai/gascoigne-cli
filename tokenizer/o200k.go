// Package tokenizer counts tokens exactly, where tokens.Estimate only
// approximates: o200k_base (the GPT-4o family's byte-pair encoding)
// reimplemented in Go over its published vocabulary, and Claude through
// Anthropic's token counting endpoint. Both cost a download or a request,
// so they serve measurements (gascoigne tokens), not the optimizer path.
package tokenizer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lukaszb/gascoigne/lazyre"
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
	resp, err := http.DefaultClient.Do(req)
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

// pieceTokens merges the piece's bytes, lowest rank first, until no pair
// is in the vocabulary.
func (b *BPE) pieceTokens(piece string) int {
	if _, ok := b.ranks[piece]; ok {
		return 1
	}
	// parts are the start offsets of the current tokens.
	parts := make([]int, len(piece)+1)
	for i := range parts {
		parts[i] = i
	}
	rank := func(i int) int {
		if i+2 >= len(parts) {
			return math.MaxInt
		}
		if r, ok := b.ranks[piece[parts[i]:parts[i+2]]]; ok {
			return r
		}
		return math.MaxInt
	}
	for len(parts) > 2 {
		best, at := math.MaxInt, -1
		for i := 0; i+2 < len(parts); i++ {
			if r := rank(i); r < best {
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
