// Package keep decides which words of a text the prose model keeps: it cuts
// the words into the chunks the model was evaluated on and scores them.
// Scoring is behind Scorer so it runs without the ONNX runtime.
package keep

import "fmt"

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

// Tokenizer encodes one word into token ids and names the sequence markers.
type Tokenizer interface {
	Word(w string) []int
	Markers() (cls, sep int)
}

// Words marks the words that have a token scoring at least threshold. Words
// are scored in chunks; a word too long for the model is kept unscored.
func Words(words []string, threshold float64, tok Tokenizer, sc Scorer) ([]bool, error) {
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
