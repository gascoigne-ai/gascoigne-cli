package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

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
			Creation    struct {
				Hour int `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// readSession reads one transcript: its main-conversation turns (subagents run
// in conversations of their own and are left out), once per message.
func readSession(reader io.Reader, name string, since time.Time) (sessionCost, error) {
	sr := sessionReader{
		session: sessionCost{ID: strings.TrimSuffix(name, ".jsonl"), Priced: true},
		since:   since,
		index:   map[string]int{},
		models:  map[string]int{},
	}
	buffered := bufio.NewReaderSize(reader, 1<<20)
	for {
		line, err := buffered.ReadBytes('\n')
		sr.addLine(line)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return sr.session, err
		}
	}
	return sr.result(), nil
}

// sessionReader holds what reading a transcript line by line has to remember.
type sessionReader struct {
	session sessionCost
	since   time.Time
	pending []string       // changes seen since the last request
	index   map[string]int // message id -> its place in turns: streamed lines repeat it
	models  map[string]int
}

// addLine routes one transcript line by its kind; lines that are not JSON are skipped.
func (sr *sessionReader) addLine(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var line usageLine
	if json.Unmarshal(raw, &line) != nil {
		return
	}
	if line.IsSidechain {
		if line.Type == "assistant" {
			sr.session.addSubagent(line, sr.since)
		}
		return
	}
	switch line.Type {
	case "attachment":
		sr.addAttachment(line)
	case "assistant":
		sr.addTurn(line)
	}
}

// addAttachment notes what an attachment injected and which prefix change it hints at.
func (sr *sessionReader) addAttachment(line usageLine) {
	ts, _ := time.Parse(time.RFC3339, line.Timestamp)
	if !sr.since.IsZero() && !ts.IsZero() && ts.Before(sr.since) {
		return
	}
	if mark := prefixMark(line.Attachment); mark != "" && !slices.Contains(sr.pending, mark) {
		sr.pending = append(sr.pending, mark)
	}
	if label, tokens := injectedText(line.Attachment); tokens > 0 {
		sr.session.inj = append(sr.session.inj, injection{label: label, tokens: tokens, before: len(sr.session.turns)})
	}
}

// addTurn counts one request of the main conversation, when it has usage and falls inside the period.
func (sr *sessionReader) addTurn(line usageLine) {
	usage := line.Message.Usage
	ctx := usage.Input + usage.CacheRead + usage.CacheCreate
	if ctx <= 0 || strings.HasPrefix(line.Message.Model, "<") {
		return
	}
	next := turn{ctx: ctx, read: usage.CacheRead, write: usage.CacheCreate, write1h: min(usage.Creation.Hour, usage.CacheCreate), input: usage.Input, output: usage.Output}
	next.ts, _ = time.Parse(time.RFC3339, line.Timestamp)
	next.model = line.Message.Model
	if sr.session.floor == 0 || ctx < sr.session.floor {
		sr.session.floor = ctx
	}
	if !sr.since.IsZero() && !next.ts.IsZero() && next.ts.Before(sr.since) {
		return // before the period: the session is cut to the part inside it
	}
	sr.record(line.Message.ID, next)
	sr.models[line.Message.Model]++
	sr.span(line)
}

// record stores a request; a streamed repeat of a message replaces the earlier line's usage.
func (sr *sessionReader) record(id string, next turn) {
	if i, ok := sr.index[id]; ok && id != "" {
		sr.session.turns[i] = next
		return
	}
	next.marks, sr.pending = sr.pending, nil
	sr.index[id] = len(sr.session.turns)
	sr.session.turns = append(sr.session.turns, next)
}

// span widens the session's time range and names its project from a request's line.
func (sr *sessionReader) span(line usageLine) {
	if ts, err := time.Parse(time.RFC3339, line.Timestamp); err == nil {
		if sr.session.Start.IsZero() || ts.Before(sr.session.Start) {
			sr.session.Start = ts
		}
		sr.session.End = maxTime(sr.session.End, ts)
	}
	if sr.session.Project == "" && line.Cwd != "" {
		sr.session.Project = filepath.Base(line.Cwd)
	}
}

// result names the model most requests used and computes what follows from the turns.
func (sr *sessionReader) result() sessionCost {
	for model, count := range sr.models {
		if count > sr.models[sr.session.Model] {
			sr.session.Model = model
		}
	}
	sr.session.finish()
	return sr.session
}

// addSubagent prices one request of a subagent (inside the period), kept apart
// from the sessions: its context is not the conversation's.
func (s *sessionCost) addSubagent(line usageLine, since time.Time) {
	u := line.Message.Usage
	if ts, err := time.Parse(time.RFC3339, line.Timestamp); err == nil && !since.IsZero() && ts.Before(since) {
		return
	}
	if model, ok := priceOf(line.Message.Model); ok && u.Input+u.CacheRead+u.CacheCreate > 0 {
		tn := turn{read: u.CacheRead, write: u.CacheCreate, write1h: min(u.Creation.Hour, u.CacheCreate), input: u.Input, output: u.Output}
		s.subUSD += tn.cost(model)
		s.subTurns++
	}
}

func maxTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
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
			s.CostUSD += t.cost(m)
		}
	}
	s.Series = downsample(s.turns, sparkPoints)
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

// downsample keeps count contexts spread over the turns (the peak of each stretch).
func downsample(turns []turn, count int) []int {
	if len(turns) == 0 {
		return []int{}
	}
	if len(turns) <= count {
		out := make([]int, len(turns))
		for i, t := range turns {
			out[i] = t.ctx
		}
		return out
	}
	out := make([]int, count)
	for i := range count {
		lo, hi := i*len(turns)/count, (i+1)*len(turns)/count
		for _, t := range turns[lo:hi] {
			out[i] = max(out[i], t.ctx)
		}
	}
	return out
}
