// Package bpe is the ModernBERT tokenizer (byte-level BPE, NFC, [CLS]/[SEP]
// template) read from a Hugging Face tokenizer.json. Words are encoded one by
// one without a prefix space, like transformers does for pre-split input.
// Added tokens such as "[CLS]" inside the text are plain text here.
package bpe

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/text/unicode/norm"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

// Tokenizer maps words to token ids.
type Tokenizer struct {
	vocab      map[string]int
	ranks      map[[2]string]int
	CLS, SEP   int
	byteToRune [256]rune
}

// gpt2Split is the ByteLevel pre-tokenizer pattern; words hold no whitespace,
// so its two lookahead alternatives are not needed.
var gpt2Split = lazyre.New(`'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+`)

// Load reads tokenizer.json.
func Load(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Model struct {
			Vocab  map[string]int `json:"vocab"`
			Merges [][2]string    `json:"merges"`
		} `json:"model"`
		Post struct {
			Special map[string]struct{ IDs []int } `json:"special_tokens"`
		} `json:"post_processor"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	t := &Tokenizer{vocab: f.Model.Vocab, ranks: make(map[[2]string]int, len(f.Model.Merges))}
	for i, m := range f.Model.Merges {
		t.ranks[m] = i
	}
	cls, sep := f.Post.Special["[CLS]"].IDs, f.Post.Special["[SEP]"].IDs
	if len(cls) != 1 || len(sep) != 1 || len(t.vocab) == 0 {
		return nil, fmt.Errorf("%s: not a ModernBERT tokenizer", path)
	}
	t.CLS, t.SEP = cls[0], sep[0]
	t.byteToRune = byteRunes()
	return t, nil
}

// byteRunes is GPT-2's byte → printable rune table.
func byteRunes() (m [256]rune) {
	n := 0
	for b := range 256 {
		if b >= '!' && b <= '~' || b >= 0xA1 && b <= 0xAC || b >= 0xAE {
			m[b] = rune(b)
			continue
		}
		m[b] = rune(256 + n)
		n++
	}
	return m
}

// Word returns the ids of one word.
func (t *Tokenizer) Word(w string) []int {
	var ids []int
	for _, piece := range gpt2Split.FindAllString(norm.NFC.String(w), -1) {
		for _, tok := range t.merge(piece) {
			ids = append(ids, t.vocab[tok])
		}
	}
	return ids
}

// merge applies the merge ranks to the byte-level form of piece.
func (t *Tokenizer) merge(piece string) []string {
	var parts []string
	for _, b := range []byte(piece) {
		parts = append(parts, string(t.byteToRune[b]))
	}
	for len(parts) > 1 {
		best, at := len(t.ranks), -1
		for i := range len(parts) - 1 {
			if r, ok := t.ranks[[2]string{parts[i], parts[i+1]}]; ok && r < best {
				best, at = r, i
			}
		}
		if at < 0 {
			break
		}
		parts = append(parts[:at], append([]string{parts[at] + parts[at+1]}, parts[at+2:]...)...)
	}
	return parts
}

// Markers returns the ids of [CLS] and [SEP].
func (t *Tokenizer) Markers() (cls, sep int) { return t.CLS, t.SEP }
