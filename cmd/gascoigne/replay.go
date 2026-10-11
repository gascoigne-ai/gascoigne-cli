package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/lukaszb/gascoigne/cost"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/mcptool"
	"github.com/lukaszb/gascoigne/tokens"
)

// cmdReplay rebuilds the requests of recorded agent sessions (Claude Code
// transcripts) and sends them through the LLM API proxy against a stand-in
// upstream: tool-result savings on real sessions, and whether every message
// an earlier request sent is sent byte-identical again (the prompt cache
// depends on it). Nothing leaves the machine; a throwaway ledger is used.
func cmdReplay(ctx context.Context, args []string) error {
	var (
		dir      string
		sessions int
		asJSON   bool
	)
	home, _ := os.UserHomeDir()
	fs := newFlags("replay")
	fs.StringVar(&dir, "dir", filepath.Join(home, ".claude", "projects"), "`directory` with session transcripts (*.jsonl), searched recursively")
	fs.IntVar(&sessions, "sessions", 10, "newest `n` transcripts to replay")
	fs.BoolVar(&asJSON, "json", false, "print JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := transcripts(dir, sessions)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "gascoigne-replay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	ledgerPath := filepath.Join(tmp, "ledger.db")
	os.Setenv(ledger.EnvPath, ledgerPath)

	up := &upstream{}
	srv := httptest.NewServer(up)
	defer srv.Close()
	var mu sync.Mutex
	proxy, closeProxy, err := llmProxy(ctx, &mu, ledgerPath, srv.URL, slog.New(slog.DiscardHandler))
	if err != nil {
		return err
	}
	defer closeProxy()

	rep := replayReport{Tools: map[string]*toolReplay{}}
	for _, f := range files {
		if err := replayFile(ctx, proxy, up, f, &rep); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		rep.Sessions++
	}
	if asJSON {
		return writeJSON(rep)
	}
	rep.print()
	return nil
}

// transcripts returns the newest n *.jsonl files under dir.
func transcripts(dir string, n int) ([]string, error) {
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, file{p, info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b file) int { return b.mod.Compare(a.mod) })
	var out []string
	for _, f := range files[:min(n, len(files))] {
		out = append(out, f.path)
	}
	return out, nil
}

// upstream stands in for the API: it keeps the body it was sent.
type upstream struct{ body []byte }

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.body, _ = io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"type":"message","content":[]}`)
}

type replayReport struct {
	Sessions int `json:"sessions"`
	Requests int `json:"requests"`
	// Compared counts messages a later request sent again unchanged by the
	// client; Unstable those the proxy sent differently the second time.
	Compared int                    `json:"compared"`
	Unstable int                    `json:"unstable"`
	Examples []string               `json:"unstable_examples,omitempty"`
	Tools    map[string]*toolReplay `json:"tools"`
	// Message input cost of every request as the client sent it and as
	// the proxy forwarded it (promptCost); system prompt and tools are the
	// same on both sides and left out.
	CostRaw float64 `json:"cost_raw_usd"`
	CostOut float64 `json:"cost_out_usd"`
	// Assumed lists sessions priced with the default model: their
	// transcript names none cost knows.
	Assumed int `json:"assumed_model_sessions,omitempty"`
}

type toolReplay struct {
	Results int `json:"results"`
	Raw     int `json:"raw_tokens"`
	Out     int `json:"out_tokens"`
	Folded  int `json:"folded"`
}

// session is the conversation rebuilt from a transcript, as the client
// sends it: consecutive lines of one role merged into one message.
type session struct {
	messages []map[string]any
	model    string
	tools    map[string]string // tool_use id → tool name
	// What the previous request sent: the client's messages and the
	// proxy's, canonical JSON.
	prevOrig, prevFwd []string
	// Latest measure per tool result id.
	results map[string]resultTokens
	price   cost.Model
	tokens  map[string]int // memo: canonical message → tokens.Estimate
}

type resultTokens struct {
	raw, out int
	folded   bool // points at an earlier result
}

type transcriptLine struct {
	Type             string `json:"type"`
	IsSidechain      bool   `json:"isSidechain"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Message          *struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content any    `json:"content"`
	} `json:"message"`
}

// replayFile sends one request per model turn that follows tool results.
func replayFile(ctx context.Context, proxy http.Handler, up *upstream, path string, rep *replayReport) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := &session{tools: map[string]string{}, results: map[string]resultTokens{}, tokens: map[string]int{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64<<20)
	lastID := ""
	for sc.Scan() {
		var l transcriptLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Message == nil || l.IsSidechain || (l.Type != "user" && l.Type != "assistant") {
			continue
		}
		content := blocks(l.Message.Content)
		switch {
		case l.IsCompactSummary:
			s.messages, s.prevOrig, s.prevFwd = []map[string]any{{"role": "user", "content": content}}, nil, nil
		case l.Type == "assistant":
			s.model = cmp.Or(l.Message.Model, s.model)
			for _, b := range content {
				if m, ok := b.(map[string]any); ok && m["type"] == "tool_use" {
					id, _ := m["id"].(string)
					name, _ := m["name"].(string)
					s.tools[id] = name
				}
			}
			if last := s.last(); last != nil && last["role"] == "assistant" && l.Message.ID == lastID {
				last["content"] = append(last["content"].([]any), content...)
				continue
			}
			if s.endsWithToolResult() {
				if err := s.send(ctx, proxy, up, path, rep); err != nil {
					return err
				}
			}
			lastID = l.Message.ID
			s.messages = append(s.messages, map[string]any{"role": "assistant", "content": content})
		default:
			if last := s.last(); last != nil && last["role"] == "user" {
				last["content"] = append(last["content"].([]any), content...)
				continue
			}
			s.messages = append(s.messages, map[string]any{"role": "user", "content": content})
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for id, t := range s.results {
		tr := rep.Tools[cmp.Or(s.tools[id], "?")]
		if tr == nil {
			tr = &toolReplay{}
			rep.Tools[cmp.Or(s.tools[id], "?")] = tr
		}
		tr.Results++
		tr.Raw += t.raw
		tr.Out += t.out
		if t.folded {
			tr.Folded++
		}
	}
	return nil
}

func (s *session) last() map[string]any {
	if len(s.messages) == 0 {
		return nil
	}
	return s.messages[len(s.messages)-1]
}

func (s *session) endsWithToolResult() bool {
	last := s.last()
	if last == nil || last["role"] != "user" {
		return false
	}
	return slices.ContainsFunc(last["content"].([]any), func(b any) bool {
		m, ok := b.(map[string]any)
		return ok && m["type"] == "tool_result"
	})
}

// blocks returns message content as a list of blocks.
func blocks(c any) []any {
	switch c := c.(type) {
	case string:
		return []any{map[string]any{"type": "text", "text": c}}
	case []any:
		return slices.Clone(c)
	}
	return nil
}

// send posts the conversation so far through the proxy, then compares what
// it forwarded with the previous request.
func (s *session) send(ctx context.Context, proxy http.Handler, up *upstream, path string, rep *replayReport) error {
	body, err := json.Marshal(map[string]any{"model": cmp.Or(s.model, "claude"), "max_tokens": 1, "messages": s.messages})
	if err != nil {
		return err
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "claude-cli/replay")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return fmt.Errorf("proxy answered %d: %s", w.Code, w.Body.String())
	}
	orig, err := canonical(body)
	if err != nil {
		return err
	}
	fwd, err := canonical(up.body)
	if err != nil {
		return err
	}
	rep.Requests++
	for i := range min(len(s.prevOrig), len(orig), len(fwd)) {
		if orig[i] != s.prevOrig[i] {
			continue
		}
		rep.Compared++
		if fwd[i] != s.prevFwd[i] {
			rep.Unstable++
			if len(rep.Examples) < 5 {
				rep.Examples = append(rep.Examples, fmt.Sprintf("%s: request %d, message %d", filepath.Base(path), rep.Requests, i))
			}
		}
	}
	if s.price.ID == "" {
		var ok bool
		if s.price, ok = cost.Lookup(s.model); !ok {
			s.price, _ = cost.Lookup(cost.AgentModels["claude"])
			rep.Assumed++
		}
	}
	rep.CostRaw += s.promptCost(orig, s.prevOrig)
	rep.CostOut += s.promptCost(fwd, s.prevFwd)
	s.prevOrig, s.prevFwd = orig, fwd
	return s.measure(body, up.body)
}

// promptCost prices a request's messages with prompt caching: the run the
// previous request sent identically is read from the cache, the rest is
// written to it.
func (s *session) promptCost(msgs, prev []string) float64 {
	read, write := 0, 0
	cached := true
	for i, m := range msgs {
		cached = cached && i < len(prev) && m == prev[i]
		n, ok := s.tokens[m]
		if !ok {
			n = tokens.Estimate(m)
			s.tokens[m] = n
		}
		if cached {
			read += n
		} else {
			write += n
		}
	}
	f := cost.Tokenizers[s.price.Tokenizer].Factor
	return f * (float64(read)*s.price.CacheRead + float64(write)*s.price.CacheWrite) / 1e6
}

// canonical re-encodes each message of a request body, so key order and
// escaping don't count as changes.
func canonical(body []byte) ([]string, error) {
	var doc struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := make([]string, len(doc.Messages))
	for i, m := range doc.Messages {
		var v any
		dec := json.NewDecoder(bytes.NewReader(m))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
		out[i] = b.String()
	}
	return out, nil
}

// measure records each tool result's tokens, sent and forwarded.
func (s *session) measure(orig, fwd []byte) error {
	before, err := toolResults(orig)
	if err != nil {
		return err
	}
	after, err := toolResults(fwd)
	if err != nil {
		return err
	}
	for id, text := range before {
		s.results[id] = resultTokens{tokens.Estimate(text), tokens.Estimate(after[id]), strings.Contains(after[id], "[↑ ")}
	}
	return nil
}

// toolResults maps tool_use ids to their result text.
func toolResults(body []byte) (map[string]string, error) {
	var doc struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, m := range doc.Messages {
		for _, b := range blocks(m.Content) {
			r, ok := b.(map[string]any)
			if !ok || r["type"] != "tool_result" {
				continue
			}
			id, _ := r["tool_use_id"].(string)
			var text []string
			for _, c := range blocks(r["content"]) {
				if t, ok := c.(map[string]any)["text"].(string); ok {
					text = append(text, t)
				}
			}
			out[id] = strings.Join(text, "\n")
		}
	}
	return out, nil
}

func (r replayReport) print() {
	fmt.Printf("replayed %d sessions, %d requests\n", r.Sessions, r.Requests)
	fmt.Printf("prompt cache: %d of %d messages sent again were byte-identical (%s)\n",
		r.Compared-r.Unstable, r.Compared, mcptool.Percent(r.Compared-r.Unstable, r.Compared))
	for _, e := range r.Examples {
		fmt.Println("  changed:", e)
	}
	fmt.Printf("message input cost, cache reads and writes priced: $%.2f → $%.2f (%s saved)\n",
		r.CostRaw, r.CostOut, mcptool.Percent(int((r.CostRaw-r.CostOut)*1e4), int(r.CostRaw*1e4)))
	if r.Assumed > 0 {
		fmt.Printf("  %d sessions name no known model: priced as %s\n", r.Assumed, cost.AgentModels["claude"])
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "tool\tresults\traw tok\tout tok\tsaved\tfolded")
	var total toolReplay
	names := slices.SortedFunc(maps.Keys(r.Tools), func(a, b string) int {
		return cmp.Or(cmp.Compare(r.Tools[b].Raw-r.Tools[b].Out, r.Tools[a].Raw-r.Tools[a].Out), cmp.Compare(a, b))
	})
	for _, n := range names {
		t := r.Tools[n]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%d\n", n, t.Results, t.Raw, t.Out, mcptool.Percent(t.Raw-t.Out, t.Raw), t.Folded)
		total.Results += t.Results
		total.Raw += t.Raw
		total.Out += t.Out
		total.Folded += t.Folded
	}
	fmt.Fprintf(w, "total\t%d\t%d\t%d\t%s\t%d\n", total.Results, total.Raw, total.Out, mcptool.Percent(total.Raw-total.Out, total.Raw), total.Folded)
	w.Flush()
}
