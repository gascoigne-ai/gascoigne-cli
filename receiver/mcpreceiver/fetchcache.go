package mcpreceiver

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lru"
)

const (
	maxCacheEntries = 32
	maxDiffLines    = 40  // a longer diff returns the full result instead
	maxDiffCells    = 4e6 // lines×lines bound of the LCS table
	diffShareLimit  = 0.5 // a diff must be this much smaller than the full text
)

// fetchEntry is what a session was last shown for one URL and query.
type fetchEntry struct {
	hash  [32]byte // status and body as fetched
	rawID string   // ledger id of the original, for the pointer
	text  string   // optimized text shown, the base of a diff
}

// fetchCache remembers the last fetch per session, URL and query, so a
// repeat can answer "unchanged" or a diff. In memory only: a lost entry just
// means a full result, never a wrong one.
type fetchCache = lru.Cache[string, fetchEntry]

func newFetchCache() *fetchCache { return lru.New[string, fetchEntry](maxCacheEntries) }

func fetchKey(session, url, query string) string {
	return strings.Join([]string{session, url, query}, "\x00")
}

func fetchHash(status int, body []byte) [32]byte {
	return sha256.Sum256(fmt.Appendf(nil, "%d\n%s", status, body))
}

// lineDiff returns the changed lines of b against a ("- old", "+ new") and
// whether it is small enough to beat sending b whole.
func lineDiff(left, right string) (string, bool) {
	x, y := strings.Split(left, "\n"), strings.Split(right, "\n")
	if float64(len(x))*float64(len(y)) > maxDiffCells {
		return "", false
	}
	// lcs[i][j]: common length of x[i:] and y[j:]
	lcs := make([][]int32, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(y)+1)
	}
	for i, v := range slices.Backward(x) {
		for j := len(y) - 1; j >= 0; j-- {
			if v == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			i++
			j++
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, "- "+x[i])
			i++
		default:
			out = append(out, "+ "+y[j])
			j++
		}
		if len(out) > maxDiffLines {
			return "", false
		}
	}
	d := strings.Join(out, "\n")
	return d, float64(len(d)) < float64(len(right))*diffShareLimit
}
