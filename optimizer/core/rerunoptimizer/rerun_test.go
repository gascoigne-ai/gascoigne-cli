package rerunoptimizer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lukaszb/gascoigne/payload"
)

// history is a fake ledger returning fixed earlier outputs.
type history struct {
	outs         []string
	session, key string
	since        time.Time
}

func (h *history) RecentOutputs(_ context.Context, session, key string, since time.Time, _ int) ([]string, error) {
	h.session, h.key, h.since = session, key, since
	return h.outs, nil
}

const failure = `--- FAIL: TestParse (0.01s)
    parse_test.go:40: config mismatch
        got:  {Name:api Port:8080 Hosts:[a b c]}
        want: {Name:api Port:9090 Hosts:[a b c]}
    parse_test.go:52: second assertion failed with a long message
`

func TestRerun(t *testing.T) {
	tests := []struct {
		name, prev, cur, want string
		typ                   string
	}{
		{
			name: "same failure folded, new one kept",
			prev: failure + "FAIL\n",
			cur:  failure + "--- FAIL: TestNew (0.00s)\n    new_test.go:9: boom\nFAIL\n",
			want: `[↑ 5 lines as in the previous run, from "--- FAIL: TestParse (0.01s)"]` + "\n--- FAIL: TestNew (0.00s)\n    new_test.go:9: boom\nFAIL\n",
			typ:  "test/go",
		},
		{
			name: "changed timing breaks only the first line",
			prev: strings.Replace(failure, "0.01s", "0.02s", 1),
			cur:  failure,
			want: "--- FAIL: TestParse (0.01s)\n" + `[↑ 4 lines as in the previous run, from "parse_test.go:40: config mismatch"]` + "\n",
			typ:  "test/go",
		},
		{
			name: "short runs stay",
			prev: "a line\nb line\n",
			cur:  "a line\nb line\n",
			want: "a line\nb line\n",
			typ:  "test/go",
		},
		{
			name: "source never folded",
			prev: failure,
			cur:  failure,
			want: failure,
			typ:  "fs/code",
		},
		{
			name: "path legend stays",
			prev: "[paths: §1=/very/long/prefix/of/the/repo]\n" + failure,
			cur:  "[paths: §1=/very/long/prefix/of/the/repo]\n" + failure,
			want: "[paths: §1=/very/long/prefix/of/the/repo]\n" + `[↑ 5 lines as in the previous run, from "--- FAIL: TestParse (0.01s)"]` + "\n",
			typ:  "test/go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			h := &history{outs: []string{tt.prev}}
			o := &Optimizer{cfg: defaults(), history: h, now: func() time.Time { return now }}
			p := payload.New(payload.CmdOutput, tt.cur)
			p.Blocks[0].Type = tt.typ
			p.Set(payload.AttrSession, "s1")
			p.Set(payload.AttrCmd, "go test ./...")
			if err := o.Optimize(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			if got := p.Text(); got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestRerunAsksForSessionAndKey(t *testing.T) {
	now := time.Now()
	h := &history{outs: []string{failure}}
	o := &Optimizer{cfg: defaults(), history: h, now: func() time.Time { return now }}
	p := payload.New(payload.CmdOutput, failure)
	p.Blocks[0].Type = "test/go"
	p.Set(payload.AttrCmd, "go test ./...")
	if err := o.Optimize(t.Context(), p); err != nil || p.Text() != failure || h.key != "" {
		t.Fatalf("no session: want untouched and no lookup, got %q (err %v)", p.Text(), err)
	}
	p.Set(payload.AttrSession, "s1")
	if err := o.Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if h.session != "s1" || h.key != p.Key() || !h.since.Equal(now.Add(-20*time.Minute)) {
		t.Fatalf("lookup %q %q %v", h.session, h.key, h.since)
	}
}
