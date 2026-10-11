# What savings are worth

`gascoigne gain` reports tokens saved, and a cost section that prices them per model.
`gascoigne tokens [file]` counts any text with every tokenizer family and prices it.

```
COST · WHAT THE SAVED TOKENS ARE WORTH
 model             tokenizer  saved tok  first read  in context
 claude-opus-5-5*  claude            3M         $15         $92
 in context: each tool result is read again by ~110 later requests (cache reads) until compaction
```

Two things decide the price of a token an agent no longer reads: how its model counts tokens,
and how often the agent pays for the same tool result.

## Counting: one estimate, a factor per tokenizer family

gascoigne counts with `tokens.Estimate` (BPE-style pieces: words, digit groups, punctuation and
whitespace runs). Models count differently, so each family gets a factor:

| Family | Models | Factor | Basis |
|---|---|---|---|
| `o200k` | OpenAI gpt-5.x, codex | 1.00 | `tokens.Estimate` is fitted to o200k: within 10% on average over 387 eval fixtures (bytes/4: 28%, up to 2.5×). cl100k counts the same on them. |
| `claude` | Opus 4.7 and later, Sonnet 5 and later, Fable | 1.58 | Measured from Claude Code transcripts: request input grows by the tool output added, so growth over the o200k count of that output is Claude's count. 160 requests on Opus 5.5 (interquartile 1.51–1.66, intercept ≈ 0), 108 on Sonnet 5 (1.63). |
| `claude-legacy` | Opus 4.6, Sonnet 4.6, Haiku 4.5 | 1.25 | Derived: Anthropic documents that the Opus 4.7 tokenizer uses 1–1.35× the tokens of earlier ones. |
| `gemini` | Gemini 2.5, 3.x | 0.79 | Google's "about 4 characters a token" over the fixtures' 3.14 characters per o200k token. Not measured. |

Claude counts this kind of output (code, logs, paths) much higher than OpenAI's tokenizers, which
is why tiktoken-based counts understate Claude costs. For exact Claude counts use Anthropic's
`count_tokens` endpoint; gascoigne stays offline and deterministic.

## Paying: first read, then every later request

Agents send the whole conversation with every request. A tool result is paid:

1. **Once at first read.** With Claude it is written to the prompt cache (1.25× input for
   the 5-minute TTL). OpenAI and Gemini cache automatically and charge plain input.
2. **Again on every later request while it stays in context**, at the cached-input price
   (Claude 0.1× input, Opus 5.5 0.05×, Fable 5.1 0.025×; OpenAI and Gemini about 0.1×), until
   compaction or the end of the session. If the cache has expired, the agent pays a fresh write.

Measured on 16 Claude Code sessions (3,148 tool results): a tool result stays in context for a
**median 110 later requests** (mean 145, deciles 17–335), and 0.8% of gaps between requests
exceed the 5-minute cache TTL. So rereads, not the first read, make up most of the cost:
on Opus 5.5 a token saved is worth $5/MTok at first read and about $31/MTok in context.

`gain` shows both: **first read** (a floor) and **in context** (with `-rereads N`, default 110).

## Which model

Runs record their model (`-model`, `GASCOIGNE_MODEL`, MCP clients that send one). Claude Code
and Gemini CLI hooks read it from the transcript the hook is given. Runs without one are priced
as the agent's default (`claude` → `claude-opus-5-5`, `codex` → `gpt-5.3-codex`,
`gemini` → `gemini-3.1-pro`) and marked `*`. Models with no known price are listed as
*not priced*.

Prices are list prices (standard tier, short context) checked 2026-10-11, in `cost/cost.go`:

- Anthropic: <https://platform.claude.com/docs/en/about-claude/pricing>
- OpenAI: <https://developers.openai.com/api/docs/pricing>
- Google: <https://ai.google.dev/gemini-api/docs/pricing>

Batch, flex, fast/priority, regional and long-context pricing are not modeled; promotional prices
(gpt-5.6-sol, Gemini 3.6/3.8 Flash until 2027-01-01) are used while they last. Model IDs match by
prefix, so variants priced differently (`-pro`, `-lite`, `-terra`) have their own rows.
