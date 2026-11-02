package drain

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func TestMinerGroups(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		same  [][2]int // pairs of line indexes that must share a cluster
		diff  [][2]int // pairs that must not
	}{
		{
			name: "variable word merges",
			lines: []string{
				"connection to host alpha failed retrying",
				"connection to host beta failed retrying",
				"connection to host gamma failed retrying",
			},
			same: [][2]int{{0, 1}, {1, 2}},
		},
		{
			name: "numbers are masked before comparing",
			lines: []string{
				"worker 12 took 340 ms",
				"worker 7 took 12 ms",
			},
			same: [][2]int{{0, 1}},
		},
		{
			name: "different length never merges",
			lines: []string{
				"cache miss for key",
				"cache miss for key in region",
			},
			diff: [][2]int{{0, 1}},
		},
		{
			name: "different meaning stays apart",
			lines: []string{
				"user alice logged in from lan",
				"disk sda reported smart failure now",
			},
			diff: [][2]int{{0, 1}},
		},
		{
			name: "low similarity stays apart",
			lines: []string{
				"user alice logged in ok",
				"user bobby logged out ok",
			},
			diff: [][2]int{{0, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{Depth: 4, Similarity: 0.6, MaxChildren: 100})
			ids := make([]int, len(tt.lines))
			for i, l := range tt.lines {
				ids[i] = m.Add("", compress.Normalize(l))
			}
			for _, p := range tt.same {
				if ids[p[0]] != ids[p[1]] {
					t.Errorf("lines %d and %d: clusters %d and %d, want same", p[0], p[1], ids[p[0]], ids[p[1]])
				}
			}
			for _, p := range tt.diff {
				if ids[p[0]] == ids[p[1]] {
					t.Errorf("lines %d and %d share cluster %d, want apart", p[0], p[1], ids[p[0]])
				}
			}
		})
	}
}

func TestMinerTagSeparates(t *testing.T) {
	m := New(Config{Depth: 4, Similarity: 0.6, MaxChildren: 100})
	a := m.Add("debug", compress.Normalize("cache warmed in shard alpha"))
	b := m.Add("", compress.Normalize("cache warmed in shard beta"))
	if a == b {
		t.Fatalf("different tags share cluster %d", a)
	}
}

func TestMinerDeterministic(t *testing.T) {
	lines := []string{"a b c d e", "a b c x e", "a b y d e", "q w e r t", "a b c d z"}
	run := func() []int {
		m := New(Config{Depth: 4, Similarity: 0.6, MaxChildren: 2})
		var ids []int
		for _, l := range lines {
			ids = append(ids, m.Add("", compress.Normalize(l)))
		}
		return ids
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("run differs at %d: %v vs %v", i, a, b)
		}
	}
}

func TestMinerBlankLine(t *testing.T) {
	m := New(Config{Depth: 4, Similarity: 0.6, MaxChildren: 100})
	if m.Add("", "") != m.Add("", "   ") {
		t.Fatal("blank lines should share a cluster")
	}
}

func TestMinerWords(t *testing.T) {
	m := New(Config{Depth: 4, Similarity: 0.6, MaxChildren: 100})
	lines := []string{
		"Started Session 1 of User dev",
		"Started Session 2 of User root",
		"Started Session 3 of User dev",
	}
	var ids []int
	for _, l := range lines {
		ids = append(ids, m.Add("", compress.Normalize(l)))
	}
	want := []string{"dev", "root", "dev"}
	for i, l := range lines {
		got := m.Words(ids[i], compress.Normalize(l))
		if len(got) != 1 || got[0] != want[i] {
			t.Errorf("Words(%q) = %v, want [%s] (numbers are not words)", l, got, want[i])
		}
	}
}

func TestMinerMaxVars(t *testing.T) {
	tests := []struct {
		name    string
		maxVars int
		same    bool
	}{
		{"one word may vary", 1, false},
		{"two words may vary", 2, true},
		{"no limit", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{Depth: 3, Similarity: 0.5, MaxChildren: 100, MaxVars: tt.maxVars})
			a := m.Add("", compress.Normalize("Stopped Avahi mDNS Stack ok now"))
			b := m.Add("", compress.Normalize("Stopped Plymouth mDNS Screen ok now"))
			if (a == b) != tt.same {
				t.Fatalf("clusters %d and %d, same = %v, want %v", a, b, a == b, tt.same)
			}
		})
	}
}
