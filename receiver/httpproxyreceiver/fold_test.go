package httpproxyreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

// numberedFrom renders lines as a Read result starting at line first.
func numberedFrom(first int, lines ...string) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%6d\t%s\n", first+i, l)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

var body = []string{
	"func Charge(ctx context.Context, o Order) error {",
	"	if o.Total <= 0 {",
	"		return ErrEmpty",
	"	}",
	"	return gateway.Charge(ctx, o.ID, o.Total)",
	"}",
}

func TestFold(t *testing.T) {
	trace := "--- FAIL: TestCharge (0.00s)\n    charge_test.go:31: gateway refused order 7\n    charge_test.go:44: want nil, got card declined\nFAIL"
	tests := []struct {
		name   string
		labels []string
		texts  []string
		want   string // the last result after folding
	}{
		{
			"re-read file",
			[]string{"Read a.go", "Read a.go"},
			[]string{numberedFrom(10, body...), numberedFrom(10, body...)},
			"[↑ L10-15 as in earlier Read a.go]",
		},
		{
			"re-read after an edit above shifts numbers",
			[]string{"Read a.go", "Read a.go"},
			[]string{numberedFrom(10, body...), numberedFrom(1, "// added") + "\n" + numberedFrom(13, body...)},
			numberedFrom(1, "// added") + "\n[↑ L13-18 = L10-15 of earlier Read a.go]",
		},
		{
			"edit inside the run keeps the changed part",
			[]string{"Read a.go", "Read a.go"},
			[]string{numberedFrom(10, body...), numberedFrom(10, body[:3]...) + "\n" + numberedFrom(13, "	}", "	log.Print(o.ID)") + "\n" + numberedFrom(15, body[4:]...)},
			"[↑ L10-13 as in earlier Read a.go]\n" + numberedFrom(14, "	log.Print(o.ID)") + "\n" + numberedFrom(15, body[4:]...),
		},
		{
			"same command output again",
			[]string{`Bash "go test ./..."`, "Bash \"go test ./...\""},
			[]string{"ok  shop/a\n" + trace, "ok  shop/b\n" + trace},
			"ok  shop/b\n[↑ 4 lines as in earlier Bash \"go test ./...\" from \"--- FAIL: TestCharge (0.00s)\"]",
		},
		{
			"numbered and plain lines never match",
			[]string{"Read a.go", "Bash \"cat a.go\""},
			[]string{numberedFrom(1, body...), strings.Join(body, "\n")},
			strings.Join(body, "\n"),
		},
		{
			"too short",
			[]string{"Bash x", "Bash x"},
			[]string{"alpha beta gamma\ndelta epsilon", "alpha beta gamma\ndelta epsilon"},
			"alpha beta gamma\ndelta epsilon",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(3, 40)
			var got string
			for i, text := range tt.texts {
				got, _ = f.fold(tt.labels[i], text, true)
			}
			if got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// Appending results never changes how earlier ones fold, so a later request
// sends the same bytes for what it shares with the one before.
func TestFoldIsPrefixStable(t *testing.T) {
	texts := []string{
		numberedFrom(1, body...),
		"go test ./...\nok  github.com/example/shop/billing 0.41s\nok  github.com/example/shop/orders 0.22s\nok  github.com/example/shop/stock 0.13s",
		numberedFrom(1, body...),
		numberedFrom(1, "// added") + "\n" + numberedFrom(2, body...),
		"go test ./...\nok  github.com/example/shop/billing 0.41s\nok  github.com/example/shop/orders 0.22s\nok  github.com/example/shop/stock 0.13s",
		numberedFrom(1, body[:2]...),
	}
	all := foldAll(texts)
	for k := 1; k <= len(texts); k++ {
		if got := foldAll(texts[:k]); !slices.Equal(got, all[:k]) {
			t.Fatalf("first %d results fold differently alone:\n%q\nvs\n%q", k, got, all[:k])
		}
	}
	if all[2] == texts[2] || all[4] == texts[4] {
		t.Fatalf("expected repeats folded: %q", all)
	}
}

func foldAll(texts []string) []string {
	f := newFolder(3, 40)
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i], _ = f.fold("Read a.go", text, true)
	}
	return out
}

// memLedger keeps originals in memory and counts events.
type memLedger struct {
	component.Ledger
	mu     sync.Mutex
	raw    map[string]string
	events []component.Event
}

func (l *memLedger) SaveRaw(_ context.Context, _ string, p *payload.Payload) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := fmt.Sprintf("%x", len(p.Text()))
	l.raw[id] = p.Text()
	return id, nil
}

func (l *memLedger) Record(_ context.Context, ev component.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
	return nil
}

func conversation(results ...string) []byte {
	msgs := []any{map[string]any{"role": "user", "content": "fix it"}}
	for i, r := range results {
		id := fmt.Sprintf("t%d", i)
		msgs = append(msgs,
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Read", "input": map[string]any{"file_path": "/src/charge.go"}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": r}}})
	}
	b, _ := json.Marshal(map[string]any{"model": "claude-opus-5-5", "messages": msgs})
	return b
}

func TestProxyFoldsRepeatedReads(t *testing.T) {
	r, _, proxy, got, _ := setup(t)
	led := &memLedger{raw: map[string]string{}}
	r.ledger = led
	long := slices.Concat(body, body, body) // big enough to beat the hint
	src := numberedFrom(1, long...)

	post(t, proxy.URL, conversation(src, src), nil)
	res := toolResult(t, *got)
	if !strings.HasPrefix(res, "[↑ L1-18 as in earlier Read /src/charge.go]\n[gascoigne: optimized ") {
		t.Fatalf("second read not folded:\n%s", res)
	}
	id := res[strings.LastIndex(res, " ")+1 : len(res)-1]
	if led.raw[id] != src || len(led.events) != 1 {
		t.Fatalf("original not kept: %q, %d events", led.raw[id], len(led.events))
	}

	// The next turn sends the same bytes for the folded read and records nothing new.
	first := resultAt(t, *got, 1)
	post(t, proxy.URL, conversation(src, src, "done"), nil)
	if again := resultAt(t, *got, 1); again != first || len(led.events) != 1 {
		t.Fatalf("prefix changed or recorded twice (%d events):\n%s", len(led.events), again)
	}
}

// resultAt is the n-th tool result of a forwarded conversation.
func resultAt(t *testing.T, body []byte, n int) string {
	t.Helper()
	var doc struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	blocks := doc.Messages[2+2*n].Content.([]any)
	return blocks[0].(map[string]any)["content"].(string)
}

func TestProxyFoldNeedsLedger(t *testing.T) {
	_, _, proxy, got, _ := setup(t)
	src := numberedFrom(1, slices.Concat(body, body, body)...)
	post(t, proxy.URL, conversation(src, src), nil)
	if res := toolResult(t, *got); res != src {
		t.Fatalf("folded without a ledger:\n%s", res)
	}
}
