// Package cost turns tokens saved into money saved, the way models count
// and agents pay for them.
//
// Each model family tokenizes differently, so a token saved by gascoigne
// (counted with tokens.Estimate, close to OpenAI's o200k) is worth a
// different number of the model's own tokens: see Tokenizers.
//
// Agents resend the whole conversation on every request, so a tool result
// is not paid once: it is written to the prompt cache (or read in full)
// the first time, then read again on every later request while it stays
// in context, until compaction or the end of the session. See Estimate.
package cost

import (
	"cmp"
	"slices"
	"strings"
)

// Tokenizer is one way models count tokens, relative to tokens.Estimate.
type Tokenizer struct {
	Name string
	// Factor is the model's tokens per tokens.Estimate token.
	Factor float64
	// Basis says where Factor comes from: measured or documented.
	Basis string
}

// Tokenizers are the families models belong to.
var Tokenizers = map[string]Tokenizer{
	"o200k": {"o200k", 1.0,
		"tokens.Estimate is calibrated on o200k: within 10% on the eval fixtures; cl100k counts the same on them (0.999)"},
	"claude": {"claude", 1.58,
		"measured: Claude Code input growth over the tool output added, 160 requests on claude-opus-5-5 (1.58, interquartile 1.51-1.66), 108 on claude-sonnet-5 (1.63)"},
	"claude-legacy": {"claude-legacy", 1.25,
		"derived: the Opus 4.7 tokenizer uses 1-1.35x the tokens of earlier Claude models"},
	"gemini": {"gemini", 0.79,
		"documented: about 4 characters a token; the fixtures average 3.14 characters per o200k token"},
}

// Model is one model's tokenizer and prices, in dollars per million tokens.
type Model struct {
	ID        string
	Tokenizer string
	Input     float64
	Output    float64
	// CacheWrite is what a token costs the first time it is cached;
	// providers with automatic caching charge plain input for it.
	CacheWrite float64
	CacheRead  float64
}

// claude prices a Claude model: 5-minute cache writes at 1.25x input,
// reads at read.
func claude(id, tok string, in, out, read float64) Model {
	return Model{ID: id, Tokenizer: tok, Input: in, Output: out, CacheWrite: in * 1.25, CacheRead: read}
}

// automatic prices a model whose provider caches by itself: no write
// premium, cached input at read. Models with no cached price pass read = in.
func automatic(id, tok string, in, out, read float64) Model {
	return Model{ID: id, Tokenizer: tok, Input: in, Output: out, CacheWrite: in, CacheRead: read}
}

// written prices a model whose provider bills cache writes: OpenAI's
// models since gpt-5.6 list them at 1.25x input.
func written(id, tok string, in, out, read float64) Model {
	return Model{ID: id, Tokenizer: tok, Input: in, Output: out, CacheWrite: in * 1.25, CacheRead: read}
}

// Models are list prices, standard tier, short context, checked 2026-10-11:
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://developers.openai.com/api/docs/pricing
//   - https://ai.google.dev/gemini-api/docs/pricing
//
// Tiers by prompt length (Haiku 5.5 over 100k, gpt-5.x over 272k, Gemini Pro
// over 200k) take the short-context price. Promotions in force: gpt-5.6-sol
// (through 2026-11-21), gemini-3.8-flash and gemini-3.6-flash (doubling on
// 2027-01-01). IDs match by prefix, so dated or suffixed variants
// (claude-opus-5-5[1m]) find their model; variants priced apart (-pro, -lite,
// -terra) need their own row.
var Models = []Model{
	claude("claude-fable-5-1", "claude", 10, 50, 0.25),
	claude("claude-fable-5", "claude", 10, 50, 1.00),
	claude("claude-mythos-5-1", "claude", 10, 50, 0.25),
	claude("claude-mythos-5", "claude", 10, 50, 1.00),
	claude("claude-opus-5-5", "claude", 4, 20, 0.20),
	claude("claude-opus-5", "claude", 5, 25, 0.50),
	claude("claude-opus-4-8", "claude", 5, 25, 0.50),
	claude("claude-opus-4-7", "claude", 5, 25, 0.50),
	claude("claude-opus-4-6", "claude-legacy", 5, 25, 0.50),
	claude("claude-opus-4-5", "claude-legacy", 5, 25, 0.50),
	claude("claude-sonnet-5-5", "claude", 2, 10, 0.10),
	claude("claude-sonnet-5", "claude", 2, 10, 0.20),
	claude("claude-sonnet-4-6", "claude-legacy", 3, 15, 0.30),
	claude("claude-sonnet-4-5", "claude-legacy", 3, 15, 0.30),
	claude("claude-haiku-5-5", "claude", 0.10, 0.50, 0.01),
	claude("claude-haiku-4-5", "claude-legacy", 1, 5, 0.10),
	written("gpt-6-astra", "o200k", 10, 50, 1.00),
	written("gpt-6.1-sol", "o200k", 2, 10, 0.10),
	written("gpt-6-sol", "o200k", 2, 10, 0.20),
	written("gpt-6-luna", "o200k", 0.10, 0.50, 0.01),
	written("gpt-5.6-sol", "o200k", 4, 20, 0.40),
	written("gpt-5.6-terra", "o200k", 2, 12, 0.20),
	written("gpt-5.6-luna", "o200k", 0.20, 1.20, 0.02),
	automatic("gpt-5.5-pro", "o200k", 30, 180, 30),
	automatic("gpt-5.5", "o200k", 5, 30, 0.50),
	automatic("gpt-5.4-pro", "o200k", 30, 180, 30),
	automatic("gpt-5.4-mini", "o200k", 0.75, 4.5, 0.075),
	automatic("gpt-5.4-nano", "o200k", 0.20, 1.25, 0.02),
	automatic("gpt-5.4", "o200k", 2.5, 15, 0.25),
	automatic("gpt-5.3-codex", "o200k", 1.75, 14, 0.175),
	automatic("gpt-5.2-pro", "o200k", 21, 168, 21),
	automatic("gpt-5.2", "o200k", 1.75, 14, 0.175),
	automatic("gpt-5.1", "o200k", 1.25, 10, 0.125),
	automatic("gpt-5-pro", "o200k", 15, 120, 15),
	automatic("gpt-5-mini", "o200k", 0.25, 2, 0.025),
	automatic("gpt-5-nano", "o200k", 0.05, 0.40, 0.005),
	automatic("gpt-5", "o200k", 1.25, 10, 0.125),
	automatic("gemini-3.8-flash", "gemini", 0.75, 3.75, 0.075),
	automatic("gemini-3.6-flash", "gemini", 0.75, 3.75, 0.075),
	automatic("gemini-3.5-flash-lite", "gemini", 0.30, 2.50, 0.03),
	automatic("gemini-3.1-flash-lite", "gemini", 0.25, 1.50, 0.025),
	automatic("gemini-3.1-pro", "gemini", 2, 12, 0.20),
	automatic("gemini-2.5-pro", "gemini", 1.25, 10, 0.125),
	automatic("gemini-2.5-flash-lite", "gemini", 0.10, 0.40, 0.01),
	automatic("gemini-2.5-flash", "gemini", 0.30, 2.50, 0.03),
}

// Lookup finds a model by exact ID, else by the longest ID prefixing it.
func Lookup(id string) (Model, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	var best Model
	for _, m := range Models {
		if strings.HasPrefix(id, m.ID) && len(m.ID) > len(best.ID) {
			best = m
		}
	}
	return best, best.ID != ""
}

// Agent defaults: the model assumed for runs recorded without one. Claude
// Code's is what its transcripts on the reference machine show; the others
// are each agent's own default.
var AgentModels = map[string]string{
	"claude":   "claude-opus-5-5",
	"codex":    "gpt-5.3-codex",
	"gemini":   "gemini-3.1-pro",
	"opencode": "claude-sonnet-5-5",
	"pi":       "claude-sonnet-5-5",
	"cursor":   "claude-sonnet-5-5",
}

// Context is how long a tool result stays in an agent's context.
type Context struct {
	// Rereads is how many later requests read a tool result again before
	// compaction or the end of the session.
	Rereads float64
	// Miss is the share of those requests that find the cache expired and
	// pay a fresh write (gaps over the 5-minute TTL).
	Miss float64
}

// AgentContext is measured on 16 Claude Code sessions (3,148 tool
// results): median 110 later requests in context (mean 145, deciles
// 17-335), 0.8% of gaps between requests over five minutes.
var AgentContext = Context{Rereads: 110, Miss: 0.008}

// Saving is what a number of tokens saved is worth with one model.
type Saving struct {
	Model       string  `json:"model"`
	Assumed     bool    `json:"assumed,omitempty"` // no model recorded: the agent's default
	Tokenizer   string  `json:"tokenizer"`
	Tokens      int     `json:"tokens"`       // saved, tokens.Estimate units
	ModelTokens int     `json:"model_tokens"` // saved, in the model's own tokens
	FirstRead   float64 `json:"first_read_usd"`
	InContext   float64 `json:"in_context_usd"` // first read plus every reread
}

// Estimate prices tokens saved (tokens.Estimate units) on model m: one
// first read (a cache write), then c.Rereads reads, c.Miss of them full
// writes again.
func Estimate(m Model, saved int, c Context) Saving {
	t := Tokenizers[m.Tokenizer]
	n := float64(saved) * t.Factor
	first := n * m.CacheWrite / 1e6
	reread := n * ((1-c.Miss)*m.CacheRead + c.Miss*m.CacheWrite) / 1e6
	return Saving{Model: m.ID, Tokenizer: t.Name, Tokens: saved, ModelTokens: int(n + 0.5),
		FirstRead: first, InContext: first + c.Rereads*reread}
}

// Total sums savings.
func Total(s []Saving) (first, inContext float64) {
	for _, x := range s {
		first += x.FirstRead
		inContext += x.InContext
	}
	return first, inContext
}

// Sort orders savings by value in context, highest first.
func Sort(s []Saving) {
	slices.SortStableFunc(s, func(a, b Saving) int { return cmp.Compare(b.InContext, a.InContext) })
}
