// Package serve is the helper's protocol loop: one JSON request per line in,
// one reply per line out, after a "ready <device>" line. Scoring is behind Scorer so
// the loop runs without the ONNX runtime.
package serve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// Words per model call, as the model was evaluated; tokens are capped by
	// the 512 positions it was trained with (two go to [CLS] and [SEP]).
	maxWords  = 350
	maxTokens = 510
)

// Scorer returns one keep score per input id of a [CLS] … [SEP] sequence.
type Scorer interface {
	Scores(ids []int64) ([]float32, error)
}

// Words encodes one word into token ids.
type Words interface {
	Word(w string) []int
}

// Tokenizer adds the sequence markers.
type Tokenizer interface {
	Words
	Markers() (cls, sep int)
}

type request struct {
	Words     []string `json:"words"`
	Threshold float64  `json:"threshold"`
}

type response struct {
	Keep  []bool `json:"keep,omitempty"`
	Error string `json:"error,omitempty"`
}

// Run answers requests until in closes.
func Run(in io.Reader, out io.Writer, tok Tokenizer, sc Scorer, device string) error {
	fmt.Fprintln(out, "ready", device)
	r := bufio.NewReaderSize(in, 1<<20)
	enc := json.NewEncoder(out)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			enc.Encode(handle(line, tok, sc))
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func handle(line []byte, tok Tokenizer, sc Scorer) response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{Error: err.Error()}
	}
	keep, err := Keep(req.Words, req.Threshold, tok, sc)
	if err != nil {
		return response{Error: err.Error()}
	}
	return response{Keep: keep}
}

// Keep marks the words that have a token scoring at least threshold. Words
// are scored in chunks; a word too long for the model is kept unscored.
func Keep(words []string, threshold float64, tok Tokenizer, sc Scorer) ([]bool, error) {
	cls, sep := tok.Markers()
	keep := make([]bool, len(words))
	for start := 0; start < len(words); {
		ids, spans := []int64{int64(cls)}, [][2]int{}
		end := start
		for ; end < len(words) && end-start < maxWords; end++ {
			w := tok.Word(words[end])
			if len(w) > maxTokens {
				keep[end] = true
				continue
			}
			if len(ids)-1+len(w) > maxTokens {
				break
			}
			spans = append(spans, [2]int{len(ids), len(ids) + len(w)})
			for _, id := range w {
				ids = append(ids, int64(id))
			}
		}
		if end == start {
			return nil, fmt.Errorf("no progress at word %d", start)
		}
		ids = append(ids, int64(sep))
		scores, err := sc.Scores(ids)
		if err != nil {
			return nil, err
		}
		if len(scores) != len(ids) {
			return nil, fmt.Errorf("%d scores for %d tokens", len(scores), len(ids))
		}
		fill(keep[start:end], spans, scores, threshold)
		start = end
	}
	return keep, nil
}

// fill sets keep for the words that own a span; words flagged by the caller
// as oversized (no span) are left as set.
func fill(keep []bool, spans [][2]int, scores []float32, threshold float64) {
	s := 0
	for i := range keep {
		if keep[i] {
			continue
		}
		for _, sc := range scores[spans[s][0]:spans[s][1]] {
			if float64(sc) >= threshold {
				keep[i] = true
				break
			}
		}
		s++
	}
}
