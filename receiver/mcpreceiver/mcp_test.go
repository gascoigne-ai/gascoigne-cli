package mcpreceiver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

// pipeline uppercases text and pretends the original was saved as "id1".
type pipeline struct{ got *payload.Payload }

func (p *pipeline) Process(_ context.Context, in *payload.Payload) (*payload.Payload, error) {
	p.got = in.Clone()
	out := in.Clone()
	if in.Attrs[payload.AttrRaw] == "true" {
		return out, nil
	}
	out.Blocks[0].Text = strings.ToUpper(out.Blocks[0].Text)
	out.Set(payload.AttrRawID, "id1")
	out.Set(payload.AttrRawTokens, "10")
	out.Set(payload.AttrOutTokens, "5")
	return out, nil
}

type ledger struct {
	component.Ledger
	failed []string
	query  component.StatsQuery
}

func (l *ledger) GetRaw(_ context.Context, id string) (string, *payload.Payload, error) {
	if id != "id1" {
		return "", nil, fmt.Errorf("raw %s: not found", id)
	}
	return "url:x", payload.New(payload.ToolResult, "original"), nil
}

func (l *ledger) MarkFailure(_ context.Context, key, _ string) error {
	l.failed = append(l.failed, key)
	return nil
}

func (l *ledger) Stats(_ context.Context, q component.StatsQuery) ([]component.Stat, error) {
	l.query = q
	return []component.Stat{{Group: "js", Runs: 2, RawTokens: 100, OutTokens: 40}}, nil
}

func connect(t *testing.T) (*mcp.ClientSession, *pipeline, *ledger) {
	t.Helper()
	pl, l := &pipeline{}, &ledger{}
	cfg := &Config{Retrieve: "retrieve", FetchTimeout: defaultTimeout, FetchMaxBytes: 1 << 20}
	serverT, clientT := mcp.NewInMemoryTransports()
	r := &Receiver{cfg: cfg, next: pl, log: slog.New(slog.DiscardHandler), ledger: l, transport: serverT, http: http.DefaultClient}
	ctx, cancel := context.WithCancel(context.Background())
	go r.Run(ctx)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); cancel() })
	return cs, pl, l
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<p>hi</p>")
	}))
	defer srv.Close()
	cs, pl, _ := connect(t)

	text, isErr := call(t, cs, "fetch", map[string]any{"url": srv.URL})
	if isErr || text != "HTTP 200 "+srv.URL+"\n<P>HI</P>\n[optimized 10→5 tokens · full: retrieve id=id1]" {
		t.Fatalf("fetch = %q (error %v)", text, isErr)
	}
	if pl.got.Attrs["mime"] != "text/html" || pl.got.Attrs[payload.AttrURL] != srv.URL {
		t.Fatalf("attrs %v", pl.got.Attrs)
	}
	if text, _ := call(t, cs, "fetch", map[string]any{"url": srv.URL, "raw": true}); !strings.HasSuffix(text, "<p>hi</p>") {
		t.Fatalf("raw fetch = %q", text)
	}
	if _, isErr := call(t, cs, "fetch", map[string]any{"url": "file:///etc/passwd"}); !isErr {
		t.Fatal("file:// accepted")
	}
}

func TestCompressRetrieveStats(t *testing.T) {
	cs, pl, l := connect(t)
	if text, _ := call(t, cs, "compress", map[string]any{"text": "abc", "cmd": "go test"}); !strings.HasPrefix(text, "ABC\n[optimized") {
		t.Fatalf("compress = %q", text)
	}
	if pl.got.Attrs[payload.AttrCmd] != "go test" || pl.got.Attrs[payload.AttrAgent] != "test" {
		t.Fatalf("attrs %v", pl.got.Attrs)
	}
	if text, _ := call(t, cs, "retrieve", map[string]any{"id": "id1"}); text != "original" || len(l.failed) != 1 {
		t.Fatalf("retrieve = %q, failures %v", text, l.failed)
	}
	if _, isErr := call(t, cs, "retrieve", map[string]any{"id": "nope"}); !isErr {
		t.Fatal("unknown id should be a tool error")
	}
	if text, _ := call(t, cs, "stats", map[string]any{}); !strings.Contains(text, "by outcome") || !strings.Contains(text, "60.0%") {
		t.Fatalf("stats overview = %q", text)
	}
	session := pl.got.Attrs[payload.AttrSession]
	if !strings.HasPrefix(session, "mcp-") || l.query.Session != session {
		t.Fatalf("session %q, stats query %+v", session, l.query)
	}
	if text, _ := call(t, cs, "stats", map[string]any{"by": "family", "format": "json"}); !strings.Contains(text, `"group":"js"`) || !strings.Contains(text, `"saved_pct":60`) {
		t.Fatalf("stats json = %q", text)
	}
	call(t, cs, "stats", map[string]any{"session": "all", "by": "route"})
	if l.query != (component.StatsQuery{By: component.ByRoute}) {
		t.Fatalf("stats query %+v", l.query)
	}
}
