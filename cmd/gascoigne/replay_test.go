package main

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReplayFile(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/eval/go/test-fail-v.txt")
	if err != nil {
		t.Fatal(err)
	}
	sandbox(t)
	up := &upstream{}
	srv := httptest.NewServer(up)
	defer srv.Close()
	ledgerFile := filepath.Join(t.TempDir(), "l.db")
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	write(t, cfg, "extensions: { ledger: { path: "+ledgerFile+" } }\n")
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	var mu sync.Mutex
	proxy, closeProxy, err := llmProxy(t.Context(), &mu, ledgerFile, srv.URL, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()

	bash := func(id string) map[string]any {
		return map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": "go test -v ./..."}}
	}
	result := func(id string) map[string]any {
		return map[string]any{"type": "tool_result", "tool_use_id": id, "content": string(fixture)}
	}
	msg := func(typ, id string, content ...any) map[string]any {
		return map[string]any{"type": typ, "message": map[string]any{"id": id, "role": typ, "model": "claude-test", "content": content}}
	}
	lines := []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": "run the tests"}},
		msg("assistant", "a1", bash("t1")),
		msg("user", "", result("t1")),
		// One assistant message written as two lines, merged again.
		msg("assistant", "a2", map[string]any{"type": "text", "text": "again"}),
		msg("assistant", "a2", bash("t2")),
		msg("user", "", result("t2")),
		{"type": "user", "isSidechain": true, "message": map[string]any{"role": "user", "content": "ignored"}},
		msg("assistant", "a3", map[string]any{"type": "text", "text": "done"}),
	}
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(append(j, '\n'))
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, b.String())

	rep := replayReport{Tools: map[string]*toolReplay{}}
	if err := replayFile(t.Context(), proxy, up, path, &rep); err != nil {
		t.Fatal(err)
	}
	// Requests before a2 and a3; the second sends the first's 3 messages again.
	if rep.Requests != 2 || rep.Compared != 3 || rep.Unstable != 0 {
		t.Errorf("requests %d, compared %d, unstable %d; want 2, 3, 0", rep.Requests, rep.Compared, rep.Unstable)
	}
	bt := rep.Tools["Bash"]
	if bt == nil || bt.Results != 2 || bt.Out >= bt.Raw {
		t.Errorf("Bash %+v, want 2 results shortened", bt)
	}
}
