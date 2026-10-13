package keep

import (
	"strings"
	"testing"
)

// fakeTok gives each word its length in tokens, ids 10+; [CLS]=1, [SEP]=2.
type fakeTok struct{}

func (fakeTok) Markers() (int, int) { return 1, 2 }
func (fakeTok) Word(w string) []int {
	ids := make([]int, len(w))
	for i := range ids {
		ids[i] = 10 + i
	}
	return ids
}

// dropper scores a token low when its id is 11 (second token of a word).
type dropper struct{ calls int }

func (d *dropper) Scores(ids []int64) ([]float32, error) {
	d.calls++
	s := make([]float32, len(ids))
	for i, id := range ids {
		if id != 11 {
			s[i] = 1
		}
	}
	return s, nil
}

func TestWords(t *testing.T) {
	// "a" has one token (kept); "bb" has ids 10,11 (token 10 scores 1: kept);
	// every word keeps a token scoring 1, so only a word whose tokens all
	// score 0 drops, which needs a threshold above 1.
	words := []string{"a", "bb", "ccc"}
	tests := []struct {
		name      string
		threshold float64
		want      []bool
	}{
		{"any subword keeps the word", 0.5, []bool{true, true, true}},
		{"above every score", 1.5, []bool{false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Words(words, tt.threshold, fakeTok{}, &dropper{})
			if err != nil {
				t.Fatal(err)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestWordsChunks(t *testing.T) {
	words := make([]string, 2*maxWords+1)
	for i := range words {
		words[i] = "a"
	}
	d := &dropper{}
	keep, err := Words(words, 0.5, fakeTok{}, d)
	if err != nil || len(keep) != len(words) || d.calls != 3 {
		t.Fatalf("calls %d, err %v", d.calls, err)
	}
}

func TestWordsTokenCap(t *testing.T) {
	long := strings.Repeat("x", maxTokens+1) // longer than the model takes: kept unscored
	keep, err := Words([]string{"a", long, "b"}, 1.5, fakeTok{}, &dropper{})
	if err != nil || keep[0] || !keep[1] || keep[2] {
		t.Fatalf("got %v, %v", keep, err)
	}
}
