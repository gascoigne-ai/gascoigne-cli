package sample

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The best matches of a query are often one line repeated with other
// numbers; taking the top n by score alone spends the whole budget on it.
func TestMMRSpreadsOverDistinctMatches(t *testing.T) {
	lines := []string{
		"proxy authentication failed for host a1",
		"proxy authentication failed for host a2",
		"proxy authentication failed for host a3",
		"proxy authentication failed for host a4",
		"unrelated line about disks",
		"the proxy requires authentication with a bearer token",
		"set proxy authentication method to basic or digest",
	}
	docs := make([][]string, len(lines))
	for i, l := range lines {
		docs[i] = Terms(l)
	}
	scores := BM25(docs, Terms("proxy authentication"))
	tests := []struct {
		name string
		got  []int
		want string
	}{
		{"top by score repeats itself", TopN(scores, 3), "0 1 2"},
		{"mmr reaches the distinct matches", MMR(scores, docs, 3, 0.5), "0 5 6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Clone(tt.got)
			slices.Sort(got)
			if s := strings.Trim(fmt.Sprint(got), "[]"); s != tt.want {
				t.Errorf("picked %s, want %s (scores %v)", s, tt.want, scores)
			}
		})
	}
}

func TestMMRKeepsEveryPositiveWhenFewer(t *testing.T) {
	docs := [][]string{{"a"}, {"b"}, {"a", "b"}}
	scores := BM25(docs, []string{"a"})
	if got := MMR(scores, docs, 10, 0.5); len(got) != 2 {
		t.Fatalf("MMR = %v, want the 2 positive scores", got)
	}
}
