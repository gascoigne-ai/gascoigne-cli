package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
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
var thresholds = []int{100_000, 150_000, 200_000, 250_000, 300_000, 400_000}

// turn is one model request of a session.
type turn struct {
	ctx, read, write, input, output int
	write1h                         int // of write, what was cached for an hour (the dearer write)
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
	// What the page says about one session besides its cost.
	SubagentUSD    float64 `json:"subagent_usd"`
	SubagentTurns  int     `json:"subagent_turns"`
	InjectedTokens int     `json:"injected_tokens"` // text added besides the prompts
	RewriteUSD     float64 `json:"rewrite_usd"`     // extra paid for cache written again
	turns          []turn
	inj            []injection
	subUSD         float64 // what subagent requests in this file cost: they run in conversations of their own
	subTurns       int
	floor          int // the smallest context of the whole session, also before the period: what a restart begins at
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

// contextDay is what the main conversations cost on one local day.
type contextDay struct {
	Day     string  `json:"day"`
	Turns   int     `json:"turns"`
	CostUSD float64 `json:"cost_usd"`
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

// cacheTTL weighs the 1-hour cache against the 5-minute one over the priced
// requests: a pause of 5 to 60 minutes rewrites the context (SavedUSD is what
// reading it from an hour cache would have saved), but every 5-minute write
// would cost more as an hour write (ExtraUSD). Advice only: nothing is changed.
type cacheTTL struct {
	Turns       int     `json:"turns"`
	PauseTokens int     `json:"pause_tokens"` // rewritten after a pause of 5 to 60 minutes
	SavedUSD    float64 `json:"saved_usd"`
	ExtraUSD    float64 `json:"extra_usd"`
	Worth       bool    `json:"worth"`
}

// The 1-hour cache is advised from ttlMinTurns requests, when it saves
// ttlMinGain times what its dearer writes cost (a margin for pauses that stay
// over the hour, and for the estimate itself).
const (
	ttlMinTurns = 200
	ttlMinGain  = 1.15
)

// add counts one session; model is the exact price of its model.
func (c *cacheTTL) add(turns []turn, model cost.Model) {
	c.Turns += len(turns)
	for _, w := range rewrites(turns) {
		if w.cause == causeFive {
			c.PauseTokens += w.tokens
			c.SavedUSD += float64(w.tokens) * (model.CacheWrite - model.CacheRead) / 1e6
		}
	}
	for _, t := range turns {
		c.ExtraUSD += float64(t.write-t.write1h) * (oneHourWrite*model.Input - model.CacheWrite) / 1e6
	}
}

func (c *cacheTTL) done() {
	c.Worth = c.Turns >= ttlMinTurns && c.ExtraUSD > 0 && c.SavedUSD >= ttlMinGain*c.ExtraUSD
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

// compactionEconomics is what the compactions the transcripts show cost and
// saved, by the same simple model as the what-if: later requests re-read the
// shorter context instead of the old one, and the compaction itself is one
// request that writes the new context to the cache plus a summary of output.
type compactionEconomics struct {
	Count      int     `json:"count"`
	MeanBefore int     `json:"mean_before"` // context of the request before it
	MeanAfter  int     `json:"mean_after"`  // context of the first request after it
	CostUSD    float64 `json:"cost_usd"`    // cache write of the new context plus the summary
	SavedUSD   float64 `json:"saved_usd"`   // cache reads avoided until the next compaction or the end
	NetUSD     float64 `json:"net_usd"`
	sumBefore  int
	sumAfter   int
}

// contextReport is the whole analysis.
type contextReport struct {
	// Billing is how Claude Code is billed here (api or subscription): a flat plan bills nothing
	// per token, so the page and the terminal output carry no dollar figures.
	Billing       string              `json:"billing"`
	Hours         int                 `json:"hours"` // the period the report covers
	Sessions      []sessionCost       `json:"sessions"`
	Turns         int                 `json:"turns"`
	CostUSD       float64             `json:"cost_usd"`
	ModelUSD      float64             `json:"model_usd"` // the baseline of the what-if, by the same simple model
	Days          []contextDay        `json:"days"`      // cost per local day, quiet days included
	Buckets       []contextBucket     `json:"buckets"`
	WhatIf        []whatIf            `json:"what_if"`
	BestWindow    int                 `json:"best_window"` // the what-if threshold that saved most, 0 when none saved anything
	Rewrites      []cacheRewrite      `json:"cache_rewrites"`
	CacheTTL      cacheTTL            `json:"cache_ttl"`
	Compactions   compactionEconomics `json:"compactions"`
	Injected      []injectedSource    `json:"injected"`
	SubagentUSD   float64             `json:"subagent_usd"` // not in the figures above: subagents have conversations of their own
	SubagentTurns int                 `json:"subagent_turns"`
	Unpriced      int                 `json:"unpriced_sessions"`
	Assumptions   []string            `json:"assumptions"`
}

// add counts the compactions of one priced session.
func (c *compactionEconomics) add(turns []turn, model cost.Model) {
	for i := 1; i < len(turns); i++ {
		prev, t := turns[i-1], turns[i]
		if !compacted(prev.ctx, t.ctx) {
			continue
		}
		later := 1 // requests that re-read the shorter context, the first included
		for j := i + 1; j < len(turns) && !compacted(turns[j-1].ctx, turns[j].ctx); j++ {
			later++
		}
		c.Count++
		c.sumBefore += prev.ctx
		c.sumAfter += t.ctx
		c.CostUSD += (float64(t.ctx)*model.CacheWrite + summaryTokens*model.Output) / 1e6
		c.SavedUSD += float64(prev.ctx-t.ctx) * float64(later) * model.CacheRead / 1e6
	}
}

// done fills what follows from the sums.
func (c *compactionEconomics) done() {
	if c.Count > 0 {
		c.MeanBefore, c.MeanAfter = c.sumBefore/c.Count, c.sumAfter/c.Count
	}
	c.NetUSD = c.SavedUSD - c.CostUSD
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

// loadContext reads the newest transcripts under dir written within period,
// and from each the turns inside it.
func loadContext(dir string, period time.Duration, now time.Time) (contextReport, error) {
	cutoff := now.Add(-period)
	files, err := recentTranscripts(dir, cutoff)
	if errors.Is(err, fs.ErrNotExist) { // no Claude Code here: an empty report, not an error
		return buildContext(nil, period), nil
	}
	if err != nil {
		return contextReport{}, err
	}
	slices.SortFunc(files, func(left, right transcriptFile) int { return right.mod.Compare(left.mod) })
	var sessions []sessionCost
	var subUSD float64
	var subTurns int
	for _, f := range files[:min(len(files), maxSessions)] {
		fh, err := os.Open(f.path)
		if err != nil {
			continue
		}
		s, err := readSession(fh, filepath.Base(f.path), cutoff)
		fh.Close()
		subUSD += s.subUSD
		subTurns += s.subTurns
		if err == nil && s.Turns > 0 {
			sessions = append(sessions, s)
		}
	}
	r := buildContext(sessions, period)
	r.SubagentUSD, r.SubagentTurns = subUSD, subTurns
	return r, nil
}

// transcriptFile is a transcript and when it was last written.
type transcriptFile struct {
	path string
	mod  time.Time
}

// recentTranscripts are the .jsonl files under dir written after cutoff.
func recentTranscripts(dir string, cutoff time.Time) ([]transcriptFile, error) {
	var files []transcriptFile
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return nil // an unreadable corner is not the whole report
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().After(cutoff) {
			files = append(files, transcriptFile{path, info.ModTime()})
		}
		return nil
	})
	return files, err
}

// claudeProjects is where Claude Code keeps its transcripts.
func claudeProjects() string {
	home := home()
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
	if !asJSON && !text && dir == claudeProjects() && canBrowse(ctx) {
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
