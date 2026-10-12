package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/cost"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// Context economics: every model request re-reads the whole conversation, so
// what a session costs is mostly turns times context, not the size of any one
// tool result. This reads Claude Code's own transcripts (which hold the usage of
// every turn, with a session id the LLM proxy never sees) and shows how big the
// conversations get, where the money goes, and what compacting earlier would
// have saved. Nothing leaves the machine and nothing is changed.

const (
	// summaryTokens is what a compaction summary is assumed to weigh: it is
	// written as output once and becomes the start of the new conversation.
	summaryTokens = 12_000
	// compactDrop: a turn whose context fell below this share of the one before
	// follows a compaction (or a /clear).
	compactDrop = 0.6
	// minContext keeps a small drop in a small conversation from counting.
	minContext = 60_000
	// maxSessions bounds the work for one report.
	maxSessions = 300
	// sparkPoints is how many points of a session's context the page draws.
	sparkPoints = 40
)

// thresholds are the contexts a what-if compacts at.
var thresholds = []int{150_000, 250_000, 400_000}

// turn is one model request of a session.
type turn struct {
	ctx, read, write, input, output int
	ts                              time.Time
	model                           string
	marks                           []string // changes the transcript records between the previous request and this one
}

// sessionCost is one conversation.
type sessionCost struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Model       string    `json:"model"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Turns       int       `json:"turns"`
	Peak        int       `json:"peak_context"`
	Compactions int       `json:"compactions"`
	CostUSD     float64   `json:"cost_usd"`
	Priced      bool      `json:"priced"`
	Series      []int     `json:"series"` // the context over the session, downsampled
	turns       []turn
	inj         []injection
	floor       int // the smallest context of the whole session, also before the period: what a restart begins at
}

// injection is text added to a conversation besides the user's prompts: what a
// hook printed, the instruction files, the skill listing. Every later request
// re-reads it.
type injection struct {
	label  string
	tokens int
	before int // requests of the session before it came in
}

// injectedSource is one source of such text, over all sessions.
type injectedSource struct {
	Source   string  `json:"source"`
	Times    int     `json:"times"`
	Sessions int     `json:"sessions"`
	Tokens   int     `json:"tokens"`        // injected in all
	PerTime  int     `json:"per_time"`      // mean size of one injection
	Rereads  int     `json:"reread_tokens"` // tokens re-read by the requests that followed
	CostUSD  float64 `json:"cost_usd"`      // written once, then read by every later request
	Share    float64 `json:"share"`         // of all the cost
}

// contextBucket is the share of the cost spent at one conversation size.
type contextBucket struct {
	Label   string  `json:"label"`
	Turns   int     `json:"turns"`
	CostUSD float64 `json:"cost_usd"`
	Share   float64 `json:"share"`
}

// cacheRewrite is the cache written again for a cause: tokens that were part of
// the previous request's context but were not read from the cache this time, so
// they were paid for at the cache-write price instead of the read price.
type cacheRewrite struct {
	Cause   string  `json:"cause"`
	Turns   int     `json:"turns"`
	Tokens  int     `json:"tokens"`
	CostUSD float64 `json:"cost_usd"` // the extra paid: write price minus read price
	Share   float64 `json:"share"`    // of all the cost
}

// Causes of a cache rewrite, told apart by what the transcript shows.
const (
	causeCompaction = "compaction or /clear"
	causeHour       = "idle for over an hour"
	causeFive       = "idle for 5 to 60 minutes"
	causeChanged    = "changed within the cache lifetime"
	causeModel      = "another model than the request before"
	causeMarked     = "changed within the cache lifetime, after a change to: "
	causeShrank     = "context shrank without a compaction: older content removed or rewritten"
	causeStart      = "only the fixed start was read: the rest of the conversation was rewritten"
)

// whatIf is the cost of compacting whenever a conversation reaches a size.
type whatIf struct {
	Threshold   int     `json:"threshold"`
	CostUSD     float64 `json:"cost_usd"`
	SavedUSD    float64 `json:"saved_usd"`
	SavedPct    float64 `json:"saved_pct"`
	Compactions int     `json:"compactions"`
	Sessions    int     `json:"sessions"` // sessions that would have compacted
}

// contextReport is the whole analysis.
type contextReport struct {
	Hours       int              `json:"hours"` // the period the report covers
	Sessions    []sessionCost    `json:"sessions"`
	Turns       int              `json:"turns"`
	CostUSD     float64          `json:"cost_usd"`
	ModelUSD    float64          `json:"model_usd"` // the baseline of the what-if, by the same simple model
	Buckets     []contextBucket  `json:"buckets"`
	WhatIf      []whatIf         `json:"what_if"`
	Rewrites    []cacheRewrite   `json:"cache_rewrites"`
	Injected    []injectedSource `json:"injected"`
	Unpriced    int              `json:"unpriced_sessions"`
	Assumptions []string         `json:"assumptions"`
}

// usageLine is the part of a transcript line this needs.
type usageLine struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	Timestamp   string          `json:"timestamp"`
	IsSidechain bool            `json:"isSidechain"`
	Attachment  json.RawMessage `json:"attachment"`
	Message     struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input       int `json:"input_tokens"`
			CacheRead   int `json:"cache_read_input_tokens"`
			CacheCreate int `json:"cache_creation_input_tokens"`
			Output      int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// readSession reads one transcript: its main-conversation turns (subagents run
// in conversations of their own and are left out), once per message.
func readSession(r io.Reader, name string, since time.Time) (sessionCost, error) {
	s := sessionCost{ID: strings.TrimSuffix(name, ".jsonl"), Priced: true}
	br := bufio.NewReaderSize(r, 1<<20)
	var pending []string      // changes seen since the last request
	index := map[string]int{} // message id -> its place in turns: streamed lines repeat it
	models := map[string]int{}
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var t usageLine
			ok := json.Unmarshal(line, &t) == nil && !t.IsSidechain
			if ok && t.Type == "attachment" {
				ts, _ := time.Parse(time.RFC3339, t.Timestamp)
				if since.IsZero() || ts.IsZero() || !ts.Before(since) {
					if mark := prefixMark(t.Attachment); mark != "" && !slices.Contains(pending, mark) {
						pending = append(pending, mark)
					}
					if label, n := injectedText(t.Attachment); n > 0 {
						s.inj = append(s.inj, injection{label: label, tokens: n, before: len(s.turns)})
					}
				}
			}
			if ok && t.Type == "assistant" {
				u := t.Message.Usage
				ctx := u.Input + u.CacheRead + u.CacheCreate
				if ctx > 0 && !strings.HasPrefix(t.Message.Model, "<") {
					tn := turn{ctx: ctx, read: u.CacheRead, write: u.CacheCreate, input: u.Input, output: u.Output}
					tn.ts, _ = time.Parse(time.RFC3339, t.Timestamp)
					tn.model = t.Message.Model
					if s.floor == 0 || ctx < s.floor {
						s.floor = ctx
					}
					if !since.IsZero() && !tn.ts.IsZero() && tn.ts.Before(since) {
						continue // before the period: the session is cut to the part inside it
					}
					if i, ok := index[t.Message.ID]; ok && t.Message.ID != "" {
						s.turns[i] = tn
					} else {
						tn.marks, pending = pending, nil
						index[t.Message.ID] = len(s.turns)
						s.turns = append(s.turns, tn)
					}
					models[t.Message.Model]++
					if ts, err := time.Parse(time.RFC3339, t.Timestamp); err == nil {
						if s.Start.IsZero() || ts.Before(s.Start) {
							s.Start = ts
						}
						s.End = maxTime(s.End, ts)
					}
					if s.Project == "" && t.Cwd != "" {
						s.Project = filepath.Base(t.Cwd)
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return s, err
		}
	}
	for m, n := range models {
		if n > models[s.Model] {
			s.Model = m
		}
	}
	s.finish()
	return s, nil
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// finish computes what follows from the turns.
func (s *sessionCost) finish() {
	s.Turns = len(s.turns)
	prev := 0
	for _, t := range s.turns {
		s.Peak = max(s.Peak, t.ctx)
		if compacted(prev, t.ctx) {
			s.Compactions++
		}
		prev = t.ctx
	}
	m, ok := priceOf(s.Model)
	s.Priced = ok
	if ok {
		for _, t := range s.turns {
			s.CostUSD += turnCost(m, t.input, t.read, t.write, t.output)
		}
	}
	s.Series = downsample(s.turns, sparkPoints)
}

// rewrites finds the turns whose cache read fell short of the previous
// request's context and says why: a compaction, a pause longer than the cache
// lives (5 minutes, or an hour for those who have it), or something that
// changed the cached start of the conversation while it was warm (an edited
// instruction file, a changed tool set, another model). Returns tokens and the
// cause by turn index.
func rewrites(turns []turn) map[int]struct {
	cause  string
	tokens int
} {
	out := map[int]struct {
		cause  string
		tokens int
	}{}
	for i := 1; i < len(turns); i++ {
		prev, t := turns[i-1], turns[i]
		missed := prev.ctx - t.read
		if missed < max(2_000, prev.ctx/20) { // under 5%: not a miss
			continue
		}
		cause := causeChanged
		switch gap := t.ts.Sub(prev.ts); {
		case compacted(prev.ctx, t.ctx):
			cause = causeCompaction
		case gap > time.Hour:
			cause = causeHour
		case gap > 5*time.Minute:
			cause = causeFive
		case t.model != "" && prev.model != "" && t.model != prev.model:
			cause = causeModel // the cache is kept per model
		case t.ctx < prev.ctx: // a request normally carries more than the one before
			cause = causeShrank
		case t.read > 0 && t.read*10 < prev.ctx*3:
			cause = causeStart
		case len(t.marks) > 0:
			cause = causeMarked + strings.Join(t.marks, ", ")
		}
		out[i] = struct {
			cause  string
			tokens int
		}{cause, missed}
	}
	return out
}

// injectedText says which source an attachment is and how many tokens of text it
// put into the conversation; none for the kinds that put nothing there.
func injectedText(raw json.RawMessage) (string, int) {
	var a struct {
		Type      string          `json:"type"`
		HookName  string          `json:"hookName"`
		HookEvent string          `json:"hookEvent"`
		Command   string          `json:"command"`
		Content   json.RawMessage `json:"content"`
		Files     json.RawMessage `json:"files"`
		Lines     json.RawMessage `json:"addedLines"`
		Blocks    json.RawMessage `json:"addedBlocks"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return "", 0
	}
	var label string
	var body json.RawMessage
	switch a.Type {
	case "hook_success":
		var text string
		if json.Unmarshal(a.Content, &text) != nil || strings.TrimSpace(text) == "" {
			return "", 0 // the hook said nothing to the model
		}
		return "hook " + a.HookEvent + ": " + shortCommand(a.Command), tokens.Estimate(text)
	case "hook_additional_context":
		if a.HookEvent == "SessionStart" { // already counted, by command, from the hook's own record
			return "", 0
		}
		if len(a.Content) == 0 {
			return "", 0
		}
		return "hook " + a.HookEvent + " (context added, hook not named)", tokens.Estimate(string(a.Content))
	case "instructions":
		label, body = "instruction files (CLAUDE.md, rules, memory)", a.Files
	case "skill_listing":
		label, body = "skill listing", a.Content
	case "agent_listing_delta":
		label, body = "agent listing", a.Lines
	case "mcp_instructions_delta":
		label, body = "MCP server instructions", a.Blocks
	default:
		return "", 0
	}
	if len(body) == 0 {
		return "", 0
	}
	return label, tokens.Estimate(string(body))
}

// prefixMark names an attachment that can change what the model is sent ahead
// of the conversation: the tool set, the instruction files, the skill, agent
// and MCP listings, the model. It is evidence beside a cache rewrite, not a proof.
func prefixMark(raw json.RawMessage) string {
	var a struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return ""
	}
	switch a.Type {
	case "deferred_tools_delta":
		return "tool set"
	case "instructions":
		return "instruction files"
	case "skill_listing":
		return "skill listing"
	case "agent_listing_delta":
		return "agent listing"
	case "mcp_instructions_delta":
		return "MCP instructions"
	case "model":
		return "model setting"
	}
	return ""
}

// shortCommand names a hook by what it runs, not by its whole command line:
// the last two parts of the first path in it, else its first words.
func shortCommand(cmd string) string {
	for _, f := range strings.Fields(cmd) {
		if strings.Contains(f, "/") && !strings.HasPrefix(f, "-") {
			parts := strings.Split(strings.Trim(f, `"'`), "/")
			return strings.Join(parts[max(len(parts)-2, 0):], "/")
		}
	}
	return oneLine(secretAssign.Regexp().ReplaceAllString(cmd, "${1}${2}…"), 40)
}

// priceOf looks a model up as transcripts name it: gateways add a provider
// prefix ("anthropic/claude-sonnet-5.5") and write versions with a dot.
func priceOf(model string) (cost.Model, bool) {
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return cost.Lookup(strings.ReplaceAll(model, ".", "-"))
}

func compacted(prev, ctx int) bool {
	return prev >= minContext && float64(ctx) < compactDrop*float64(prev)
}

// turnCost prices one request at list prices; cache writes at the 5-minute
// price. Tokens are priced per million.
func turnCost(m cost.Model, input, read, write, output int) float64 {
	return (float64(input)*m.Input + float64(read)*m.CacheRead + float64(write)*m.CacheWrite + float64(output)*m.Output) / 1e6
}

// downsample keeps n contexts spread over the turns (the peak of each stretch).
func downsample(turns []turn, n int) []int {
	if len(turns) == 0 {
		return []int{}
	}
	if len(turns) <= n {
		out := make([]int, len(turns))
		for i, t := range turns {
			out[i] = t.ctx
		}
		return out
	}
	out := make([]int, n)
	for i := range n {
		lo, hi := i*len(turns)/n, (i+1)*len(turns)/n
		for _, t := range turns[lo:hi] {
			out[i] = max(out[i], t.ctx)
		}
	}
	return out
}

// simulate prices the session as if it had been compacted whenever its context
// passed limit (0: never), with the same simple model for every limit, so the
// numbers compare with each other: a request pays cache reads for what was
// already cached, cache writes for what is new, and its output. A compaction
// is one request over the full context that writes the summary; the next
// request writes the new, short conversation. It returns the cost and how many
// compactions that took.
func simulate(turns []turn, limit, base int, m cost.Model) (usd float64, compactions int) {
	if len(turns) == 0 {
		return 0, 0
	}
	if base == 0 { // what every conversation starts with: system prompt, tools, instructions
		base = turns[0].ctx
		for _, t := range turns {
			base = min(base, t.ctx)
		}
	}
	if base+summaryTokens >= limit {
		limit = 0 // compacting could not make it smaller: nothing to simulate
	}
	cur, cached := 0, turns[0].read // a conversation cut at the start of the period begins with what was already cached
	prevOrig := 0
	for i, t := range turns {
		switch {
		case i == 0:
			cur = t.ctx
		case compacted(prevOrig, t.ctx):
			cur, cached = t.ctx, 0 // the real conversation started over here
		default:
			cur += max(t.ctx-prevOrig, 0)
		}
		prevOrig = t.ctx
		if limit > 0 && cur > limit {
			usd += (float64(cached)*m.CacheRead + float64(summaryTokens)*m.Output) / 1e6
			compactions++
			cur, cached = base+summaryTokens, 0
		}
		usd += turnCost(m, 0, cached, cur-cached, t.output)
		cached = cur
	}
	return usd, compactions
}

// buildContext assembles the report from sessions already read.
func buildContext(sessions []sessionCost, period time.Duration) contextReport {
	r := contextReport{Hours: int(period.Hours()), Sessions: sessions, Buckets: []contextBucket{
		{Label: "under 100k"}, {Label: "100k to 200k"}, {Label: "200k to 400k"}, {Label: "over 400k"},
	}}
	if r.Sessions == nil {
		r.Sessions = []sessionCost{}
	}
	slices.SortFunc(r.Sessions, func(a, b sessionCost) int { return b.End.Compare(a.End) })
	byCause := map[string]*cacheRewrite{}
	bySource := map[string]*injectedSource{}
	sessionsOf := map[string]map[string]bool{}
	wi := make([]whatIf, len(thresholds))
	for i, t := range thresholds {
		wi[i].Threshold = t
	}
	for _, s := range r.Sessions {
		r.Turns += s.Turns
		if !s.Priced {
			r.Unpriced++
			continue
		}
		r.CostUSD += s.CostUSD
		m, _ := priceOf(s.Model)
		base, _ := simulate(s.turns, 0, s.floor, m)
		r.ModelUSD += base
		for i, t := range thresholds {
			usd, n := simulate(s.turns, t, s.floor, m)
			wi[i].CostUSD += usd
			wi[i].SavedUSD += base - usd
			wi[i].Compactions += n
			if n > 0 {
				wi[i].Sessions++
			}
		}
		for _, in := range s.inj {
			src := bySource[in.label]
			if src == nil {
				src = &injectedSource{Source: in.label}
				bySource[in.label] = src
				sessionsOf[in.label] = map[string]bool{}
			}
			later := max(s.Turns-in.before, 0)
			src.Times++
			src.Tokens += in.tokens
			src.Rereads += in.tokens * later
			src.CostUSD += float64(in.tokens) * (m.CacheWrite + float64(later)*m.CacheRead) / 1e6
			sessionsOf[in.label][s.ID] = true
		}
		for _, w := range rewrites(s.turns) {
			rw := rewriteOf(byCause, w.cause)
			rw.Turns++
			rw.Tokens += w.tokens
			rw.CostUSD += float64(w.tokens) * (m.CacheWrite - m.CacheRead) / 1e6
		}
		for _, t := range s.turns {
			b := &r.Buckets[bucketOf(t.ctx)]
			b.Turns++
			b.CostUSD += turnCost(m, t.input, t.read, t.write, t.output)
		}
	}
	for i := range r.Buckets {
		if r.CostUSD > 0 {
			r.Buckets[i].Share = r.Buckets[i].CostUSD / r.CostUSD
		}
	}
	for label, src := range bySource {
		src.Sessions = len(sessionsOf[label])
		src.PerTime = src.Tokens / max(src.Times, 1)
		if r.CostUSD > 0 {
			src.Share = src.CostUSD / r.CostUSD
		}
		r.Injected = append(r.Injected, *src)
	}
	slices.SortFunc(r.Injected, func(a, b injectedSource) int {
		return cmp.Or(cmp.Compare(b.CostUSD, a.CostUSD), cmp.Compare(a.Source, b.Source))
	})
	if r.Injected == nil {
		r.Injected = []injectedSource{}
	}
	for _, cause := range []string{causeFive, causeHour, causeChanged, causeCompaction} {
		rewriteOf(byCause, cause) // the usual causes always show, even at zero
	}
	for _, rw := range byCause {
		if r.CostUSD > 0 {
			rw.Share = rw.CostUSD / r.CostUSD
		}
		r.Rewrites = append(r.Rewrites, *rw)
	}
	slices.SortFunc(r.Rewrites, func(a, b cacheRewrite) int {
		return cmp.Or(cmp.Compare(b.CostUSD, a.CostUSD), cmp.Compare(a.Cause, b.Cause))
	})
	for i := range wi {
		if r.ModelUSD > 0 {
			wi[i].SavedPct = 100 * wi[i].SavedUSD / r.ModelUSD
		}
	}
	r.WhatIf = wi
	r.Assumptions = []string{
		"Dollars are what the tokens would cost at API list prices (5-minute cache writes); a subscription does not bill per token, and 1-hour cache writes cost more.",
		fmt.Sprintf("A compaction is assumed to cost one request over the whole context plus a %dk-token summary; later turns grow as they did.", summaryTokens/1000),
		"Compacting saves tokens, not detail: the summary forgets what it leaves out. Treat the saving as an upper bound on what is worth giving up.",
		"Only main conversations are counted; subagents run in conversations of their own.",
		"Added text is priced as written once to the cache and read by every later request of the session; what a hook printed is named by the command it ran, as the transcript records it.",
		"A cache rewrite is context that the previous request carried but this one did not read from the cache; the cost shown is the extra over a read. The causes are read from the pauses between requests, so \"changed within the cache lifetime\" means the cached start of the conversation changed while it was warm (an edited instruction file, a changed tool set, another model); a transcript cannot say which.",
	}
	return r
}

func rewriteOf(m map[string]*cacheRewrite, cause string) *cacheRewrite {
	if m[cause] == nil {
		m[cause] = &cacheRewrite{Cause: cause}
	}
	return m[cause]
}

func bucketOf(ctx int) int {
	switch {
	case ctx < 100_000:
		return 0
	case ctx < 200_000:
		return 1
	case ctx < 400_000:
		return 2
	}
	return 3
}

// loadContext reads the newest transcripts under dir written within period,
// and from each the turns inside it.
func loadContext(dir string, period time.Duration, now time.Time) (contextReport, error) {
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	cutoff := now.Add(-period)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil // an unreadable corner is not the whole report
		}
		if d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.ModTime().After(cutoff) {
			files = append(files, file{p, info.ModTime()})
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) { // no Claude Code here: an empty report, not an error
		return buildContext(nil, period), nil
	}
	if err != nil {
		return contextReport{}, err
	}
	slices.SortFunc(files, func(a, b file) int { return b.mod.Compare(a.mod) })
	var sessions []sessionCost
	for _, f := range files[:min(len(files), maxSessions)] {
		fh, err := os.Open(f.path)
		if err != nil {
			continue
		}
		s, err := readSession(fh, filepath.Base(f.path), cutoff)
		fh.Close()
		if err == nil && s.Turns > 0 {
			sessions = append(sessions, s)
		}
	}
	return buildContext(sessions, period), nil
}

// claudeProjects is where Claude Code keeps its transcripts.
func claudeProjects() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// cmdContext opens the page; the table is the fallback for a terminal without
// a browser.
func cmdContext(ctx context.Context, args []string) error {
	flags := newFlags("context")
	var (
		asJSON, text bool
		dir          string
	)
	since := flags.String("since", "14d", "how far back to read transcripts: `24h`, 7d, 4w, ...")
	open := true
	flags.BoolVar(&asJSON, "json", false, "print the report as JSON")
	flags.BoolVar(&text, "text", false, "print the table instead of opening the page")
	flags.BoolVar(&open, "open", true, "open the page in a browser; false only prints its address")
	flags.StringVar(&dir, "dir", claudeProjects(), "`directory` with Claude Code transcripts (*.jsonl), searched recursively")
	if err := flags.Parse(args); err != nil {
		return err
	}
	d, err := parseSince(*since)
	if err != nil {
		return err
	}
	if !asJSON && !text && dir == claudeProjects() && canBrowse() {
		link := fmt.Sprintf("http://%s/context?since=%s", serverAddr(), *since)
		fmt.Println(link)
		if open {
			openBrowser(link)
		}
		return nil
	}
	report, err := loadContext(dir, d, time.Now())
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(report)
	}
	printContext(os.Stdout, report)
	return nil
}

// periodLabel says a number of hours the way a person would: "24 hours", "14 days".
func periodLabel(hours int) string {
	if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d days", hours/24)
}

// printContext writes the report as text, the terminal form of the page.
func printContext(w io.Writer, r contextReport) {
	if len(r.Sessions) == 0 {
		fmt.Fprintf(w, "No Claude Code transcripts with usage in the last %s (looked in %s).\n", periodLabel(r.Hours), claudeProjects())
		return
	}
	fmt.Fprintf(w, "%d sessions, %d requests, about %s at API list prices, last %s\n\n", len(r.Sessions), r.Turns, dollars(r.CostUSD), periodLabel(r.Hours))
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CONTEXT OF A REQUEST\tREQUESTS\tCOST\tSHARE")
	for _, b := range r.Buckets {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%.0f%%\n", b.Label, b.Turns, dollars(b.CostUSD), 100*b.Share)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nContext written to the cache again instead of read:")
	tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CAUSE\tREQUESTS\tTOKENS\tEXTRA COST\tSHARE")
	for _, x := range r.Rewrites {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%.1f%%\n", x.Cause, x.Turns, human(x.Tokens), dollars(x.CostUSD), 100*x.Share)
	}
	tw.Flush()
	if len(r.Injected) > 0 {
		fmt.Fprintln(w, "\nText added to conversations besides your prompts (every later request re-reads it):")
		tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "SOURCE\tTIMES\tPER TIME\tCOST\tSHARE")
		for _, x := range r.Injected[:min(len(r.Injected), 8)] {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%.1f%%\n", x.Source, x.Times, human(x.PerTime), dollars(x.CostUSD), 100*x.Share)
		}
		tw.Flush()
	}
	fmt.Fprintln(w, "\nIf conversations had been compacted when they reached:")
	tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CONTEXT\tSESSIONS AFFECTED\tCOMPACTIONS\tSAVED")
	for _, x := range r.WhatIf {
		fmt.Fprintf(tw, "%dk\t%d\t%d\t%s (%.0f%%)\n", x.Threshold/1000, x.Sessions, x.Compactions, dollars(x.SavedUSD), x.SavedPct)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nLargest sessions:")
	top := slices.Clone(r.Sessions)
	slices.SortFunc(top, func(a, b sessionCost) int { return cmp.Compare(b.CostUSD, a.CostUSD) })
	tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tSTART\tREQUESTS\tPEAK\tCOMPACTIONS\tCOST")
	for _, s := range top[:min(len(top), 8)] {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%d\t%s\n", cmp.Or(s.Project, "-"), s.Start.Local().Format("2006-01-02"), s.Turns, human(s.Peak), s.Compactions, dollars(s.CostUSD))
	}
	tw.Flush()
	for _, a := range r.Assumptions {
		fmt.Fprintln(w, "\n"+a)
	}
}
