# LLM API proxy (`receiver/httpproxyreceiver`, `cmd/gascoigne/llmproxy.go`)

## Serving

- Mounted in the background server under `/v1/`: one process, one port, base URL
  `http://127.0.0.1:7878`.
- The receiver is an `http.Handler` (`Run` is a no-op), built from the user's config
  (`receivers.httpproxy`, pipeline `proxy` via `optimizerService`). Payloads run one at a time
  under the warm services' lock.
- On a new build the server waits until no proxied request is in flight (`waitIdle`, ≤ 10 min)
  before restarting.

## APIs

| API | Matched by | Notes |
|---|---|---|
| Anthropic `/v1/messages*` | path, or `anthropic-version`/`x-api-key` headers | |
| OpenAI `/chat/completions`, `/responses` | path | |
| Gemini `/v1beta/models/<m>:generateContent\|:streamGenerateContent` | path (also mounted at `/v1beta/`), or `x-goog-api-key` | `functionResponse.response.output\|content` answers the call with its id, else the oldest open call of its name; model from the path |

- **No session id.** None of the APIs carries the agent's session, so proxy events and `usage` rows
  have an empty `session` (agent from the User-Agent, model from the body). Per-session reports
  (`gain -session`, the dashboard session filter) leave the proxy's runs out; they show as the "-"
  group, mixed with command-line runs started without `-session`. Fixing it needs an id from the
  agent (a header it can be told to send); none is known today.
- Other paths and every header pass through. `anthropic-beta` carries the OAuth capability, so a
  claude.ai login keeps working.
- Responses stream through (`FlushInterval -1`).
- Any parse problem → the body is forwarded unchanged. `X-Gascoigne-Raw: 1` → untouched.

## Tool results

- One payload per tool result, with attrs from the matching call (`attrsOf`):
  - built-in tools are described as the shell command that prints the same thing: Read →
    `cat <path>` (or `sed -n <offset>,$p` for slices), Grep → `rg -n|-l|-c`, Glob/list → `find`
  - `query`/`prompt` → query attr; url; tool; model from the body; agent from the User-Agent
- `skip` tools, `is_error` results and hinted output stay untouched.
- Read's `N\t` prefixes are split off, the text is optimized like `cat`, and kept lines get their
  original numbers back (`renumber`).
- **Prompt cache**: results are cached by content hash (LRU `cache_entries`), so every later
  request sends the same bytes. Optimizers are deterministic, so a restart recomputes them.

## Fold (`fold.go`, per request)

After shortening, runs of ≥ `fold_min_lines` lines / `fold_min_chars` bytes that an earlier
result of the same request shows become one pointer: `[↑ L13-18 = L10-15 of earlier Read a.go]`.
Read lines match by text, with a uniform line-number shift.
- Recomputed for each request: a result depends only on earlier ones, so it is prefix-stable and
  leaves no dangling pointers when clients clear old results.
- The original is saved via `SaveRaw` (content-addressed id) + an event (route `fold`); savings
  are memoized. Needs `ledger` (looked up in `Start`).

## Other request parts

- **Tool schemas** (`tools.go`, `compact_tools`, `internal/compress/schema`): titles restating names and
  `$schema`/`$id` are dropped in every API's shape; one event per distinct tool set (route `tools`).
- **Instructions** (`prompts.go`): system prompts, `<system-reminder>`s in user text, chat/responses
  system/developer messages and `instructions`, Gemini `systemInstruction`. Each distinct block
  records one unchanged event (route `prompt`, command `system prompt`/`CLAUDE.md`/`skills`/
  `reminder`), so `gain -by command` shows what instruction text costs.
  - The text is kept as the run's original (`gascoigne raw <id>`) and pruned with `raw_ttl` like
    other originals.
  - The dashboard shows only its size: `/api/raw|out` answer 403 for `prompt:` keys.
- **Skills** (`skills.go`): invoked skill bodies (user text starting
  `Base directory for this skill:`) are recorded once per body with their text kept (route
  `skill`, command `skill <name>`).
  - With `skill_code_lines`, longer code blocks keep their head plus
    `… N more lines: gascoigne raw <id> -lines a-b`. HTML comments go.
  - Measured: a 30k-token bundled skill is prose and tables (−0.3%).
- **Images** (`images.go`, `max_image_pixels`, 0 = off): base64 PNG/JPEG blocks in tool results
  above the cap are box-downscaled (deterministic, cached by hash). The original is saved (`raw`)
  and the event counted in image tokens (pixels/750, route `image`).

## Usage metering (`usage.go`)

- 2xx response bodies pass through a meter: SSE `data:` lines mentioning usage are decoded, JSON
  bodies ≤ 8 MB at the end.
- Per API reader, fields are max-merged across events; cached input is split out (OpenAI and
  Gemini count it in input). Anthropic's `cache_creation.ephemeral_1h_input_tokens` becomes
  `component.Usage.CacheWrite1h` (a part of `CacheWrite`; the one change to `component/` for this, approved
  2026-10-12), stored in `usage.cache_write1h` and priced at twice the input price by `gain` and the context report;
  the other providers leave it 0.
- On close, one `component.Usage` goes to the ledger's optional `component.UsageRecorder`
  (table `usage`, no session).
- `gain` section "api usage", priced by `cost.Lookup`; JSON `api_usage`; `reset` filters apply. Each row also
  carries the mean and peak context of a request (input + cache read + cache write: the conversation it
  re-read), and `UsageDays` sums per UTC day (`gain` table "api requests by day" from two days on, JSON
  `api_days`, the dashboard panels "API requests by day|model" over `GET /api/usage`). No session id: see above.

## Registration with agents

The server is started first (`serverUp`). Then:

| Agent | Proxied when | How |
|---|---|---|
| claude (`claudeProxyVar`) | `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`, or `GASCOIGNE_PROXY=on`. A claude.ai login stays direct (terms of use) | `ANTHROPIC_BASE_URL` |
| claude on Bedrock | `AWS_BEARER_TOKEN_BEDROCK` | `ANTHROPIC_BEDROCK_BASE_URL=…/bedrock/<region>`; route `bedrock.go`, region validated, upstream template `bedrock`. SigV4 stays direct: it signs the body |
| codex | `OPENAI_API_KEY` | a `model_providers.gascoigne` (`wire_api=responses`) |
| opencode | each provider only with its API key | provider `baseURL`s |
| gemini | `GEMINI_API_KEY`. A Google login stays direct | `GOOGLE_GEMINI_BASE_URL` |

- Never on Vertex or Foundry, nor over a user's own base URL.
- `install -proxy` refuses in the same cases. `install -proxy claude` writes the env for good
  (SessionStart starts the server); unwrap removes it.

## Replay (`cmd/gascoigne/replay.go`)

`gascoigne replay` rebuilds Claude Code transcripts (`~/.claude/projects`, newest first) into one
request per model turn after tool results:
- lines of one role are merged, sidechains skipped, compaction restarts the conversation
- requests go through `llmProxy` against an in-process upstream with a throwaway ledger
- reports: tokens per tool; message input cost as sent and as forwarded (`cost` prices: the
  prefix the previous request sent identically is read from the cache, the rest written); how
  many re-sent messages the proxy forwarded byte-identical (canonical JSON)
