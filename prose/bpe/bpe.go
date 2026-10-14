// Package bpe is the BPE tokenizer of the prose models: ModernBERT's byte-level
// kind (NFC, [CLS]/[SEP]) and the SentencePiece kind ("▁" word starts, byte
// fallback). Words are tokenized one by one, as they would sit in a text
// joined by spaces, so a token never crosses a word.
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
	added      map[string]int // tokens the text is matched against before splitting, such as <td>
	metaspace  bool           // SentencePiece style: "▁" marks a word start, unknown characters fall back to bytes
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
		Added []struct {
			ID      int
			Content string
		} `json:"added_tokens"`
		Pre struct {
			Type string `json:"type"`
		} `json:"pre_tokenizer"`
		Post struct {
			Single []struct {
				Special *struct{ ID string } `json:"SpecialToken"`
			} `json:"single"`
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
	// The markers are the template's first and last special tokens.
	var marks []int
	for _, part := range f.Post.Single {
		if part.Special != nil {
			marks = append(marks, f.Post.Special[part.Special.ID].IDs...)
		}
	}
	if len(marks) != 2 || len(t.vocab) == 0 {
		return nil, fmt.Errorf("%s: not a BPE tokenizer with a start and end marker", path)
	}
	t.CLS, t.SEP = marks[0], marks[1]
	t.metaspace = f.Pre.Type == "Metaspace"
	t.added = make(map[string]int, len(f.Added))
	for _, a := range f.Added {
		t.added[a.Content] = a.ID
	}
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
	if t.metaspace {
		return t.spmWord(w)
	}
	var ids []int
	for _, piece := range gpt2Split.FindAllString(norm.NFC.String(w), -1) {
		for _, tok := range t.merge(piece) {
			ids = append(ids, t.vocab[tok])
		}
	}
	return ids
}

// spmWord tokenizes a word as it would sit in a space-joined text: "▁" in
// front, merges over characters, and a character the vocabulary lacks becomes
// one <0xHH> token per byte. No Unicode normalization.
func (t *Tokenizer) spmWord(w string) []int {
	if id, ok := t.added[w]; ok { // only a whole word, not one inside a longer word
		return []int{id}
	}
	var parts []string
	for _, r := range "▁" + w {
		parts = append(parts, string(r))
	}
	var ids []int
	for _, tok := range mergeRanks(t.ranks, parts) {
		if id, ok := t.vocab[tok]; ok {
			ids = append(ids, id)
			continue
		}
		for _, b := range []byte(tok) {
			ids = append(ids, t.vocab[fmt.Sprintf("<0x%02X>", b)])
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
	return mergeRanks(t.ranks, parts)
}

// mergeRanks joins adjacent parts, lowest rank first, until no pair has one.
func mergeRanks(ranks map[[2]string]int, parts []string) []string {
	for len(parts) > 1 {
		best, at := len(ranks), -1
		for i := range len(parts) - 1 {
			if r, ok := ranks[[2]string{parts[i], parts[i+1]}]; ok && r < best {
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
