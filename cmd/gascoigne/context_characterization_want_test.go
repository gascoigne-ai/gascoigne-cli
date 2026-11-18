package main

// Recorded from the report code before context.go was split by job.
var characterizationWant = map[string]string{
	"json": `{
 "billing": "api",
 "hours": 336,
 "sessions": [
  {
   "id": "s1",
   "project": "shop",
   "model": "claude-sonnet-5-5",
   "start": "2026-10-10T10:01:00Z",
   "end": "2026-10-10T11:41:00Z",
   "turns": 10,
   "peak_context": 210010,
   "compactions": 1,
   "cost_usd": 1.0774,
   "priced": true,
   "series": [
    40010,
    60010,
    80010,
    80010,
    150010,
    210010,
    210010,
    40010,
    42010,
    43010
   ],
   "subagent_usd": 0.01904,
   "subagent_turns": 1,
   "injected_tokens": 40,
   "rewrite_usd": 0.828072
  }
 ],
 "turns": 10,
 "cost_usd": 1.0774,
 "model_usd": 0.7292580000000001,
 "days": [
  {
   "day": "2026-10-10",
   "turns": 10,
   "cost_usd": 1.0774
  }
 ],
 "buckets": [
  {
   "label": "under 100k",
   "turns": 7,
   "cost_usd": 0.6703399999999999,
   "share": 0.6221830332281418
  },
  {
   "label": "100k to 200k",
   "turns": 1,
   "cost_usd": 0.18702,
   "share": 0.17358455541117507
  },
  {
   "label": "200k to 400k",
   "turns": 2,
   "cost_usd": 0.22004,
   "share": 0.20423241136068315
  },
  {
   "label": "over 400k",
   "turns": 0,
   "cost_usd": 0,
   "share": 0
  }
 ],
 "what_if": [
  {
   "threshold": 100000,
   "cost_usd": 0.8787079999999999,
   "saved_usd": -0.14944999999999986,
   "saved_pct": -20.49343305112866,
   "compactions": 2,
   "sessions": 1
  },
  {
   "threshold": 150000,
   "cost_usd": 0.784683,
   "saved_usd": -0.05542499999999995,
   "saved_pct": -7.600190878948184,
   "compactions": 1,
   "sessions": 1
  },
  {
   "threshold": 200000,
   "cost_usd": 0.8134830000000001,
   "saved_usd": -0.084225,
   "saved_pct": -11.549410496696641,
   "compactions": 1,
   "sessions": 1
  },
  {
   "threshold": 250000,
   "cost_usd": 0.7292580000000001,
   "saved_usd": 0,
   "saved_pct": 0,
   "compactions": 0,
   "sessions": 0
  },
  {
   "threshold": 300000,
   "cost_usd": 0.7292580000000001,
   "saved_usd": 0,
   "saved_pct": 0,
   "compactions": 0,
   "sessions": 0
  },
  {
   "threshold": 400000,
   "cost_usd": 0.7292580000000001,
   "saved_usd": 0,
   "saved_pct": 0,
   "compactions": 0,
   "sessions": 0
  }
 ],
 "best_window": 0,
 "cache_rewrites": [
  {
   "cause": "compaction or /clear",
   "turns": 1,
   "tokens": 210010,
   "cost_usd": 0.504024,
   "share": 0.46781511045108604
  },
  {
   "cause": "idle for over an hour",
   "turns": 1,
   "tokens": 80010,
   "cost_usd": 0.192024,
   "share": 0.17822906998329313
  },
  {
   "cause": "idle for 5 to 60 minutes",
   "turns": 1,
   "tokens": 55010,
   "cost_usd": 0.132024,
   "share": 0.1225394468164099
  },
  {
   "cause": "changed within the cache lifetime",
   "turns": 0,
   "tokens": 0,
   "cost_usd": 0,
   "share": 0
  }
 ],
 "cache_ttl": {
  "turns": 10,
  "pause_tokens": 55010,
  "saved_usd": 0.132024,
  "extra_usd": 0.597,
  "worth": false
 },
 "compactions": {
  "count": 1,
  "mean_before": 210010,
  "mean_after": 40010,
  "cost_usd": 0.220025,
  "saved_usd": 0.051,
  "net_usd": -0.169025
 },
 "injected": [
  {
   "source": "instruction files (CLAUDE.md, rules, memory)",
   "times": 1,
   "sessions": 1,
   "tokens": 15,
   "per_time": 15,
   "reread_tokens": 150,
   "cost_usd": 0.0000525,
   "share": 0.000048728420271022836
  },
  {
   "source": "skill listing",
   "times": 1,
   "sessions": 1,
   "tokens": 10,
   "per_time": 10,
   "reread_tokens": 80,
   "cost_usd": 0.000033,
   "share": 0.00003062929274178578
  },
  {
   "source": "hook UserPromptSubmit: hooks/nudge.sh",
   "times": 1,
   "sessions": 1,
   "tokens": 8,
   "per_time": 8,
   "reread_tokens": 80,
   "cost_usd": 0.000028,
   "share": 0.000025988490811212178
  },
  {
   "source": "hook PostToolUse (context added, hook not named)",
   "times": 1,
   "sessions": 1,
   "tokens": 7,
   "per_time": 7,
   "reread_tokens": 42,
   "cost_usd": 0.0000217,
   "share": 0.000020141080378689438
  }
 ],
 "subagent_usd": 0.01904,
 "subagent_turns": 1,
 "unpriced_sessions": 0,
 "assumptions": [
  "Dollars are what the tokens would cost at API list prices, cache writes at the 5-minute or 1-hour rate the transcript records; a subscription does not bill per token.",
  "A compaction is assumed to cost one request over the whole context plus a 12k-token summary; later turns grow as they did.",
  "Compacting saves tokens, not detail: the summary forgets what it leaves out. Treat the saving as an upper bound on what is worth giving up.",
  "Sessions are main conversations; what subagents cost is added up apart, since they run in conversations of their own.",
  "Added text is priced as written once to the cache and read by every later request of the session; what a hook printed is named by the command it ran, as the transcript records it.",
  "A cache rewrite is context that the previous request carried but this one did not read from the cache; the cost shown is the extra over a read. The causes are read from the pauses between requests, so \"changed within the cache lifetime\" means the cached start of the conversation changed while it was warm (an edited instruction file, a changed tool set, another model); a transcript cannot say which."
 ]
}`,
	"text": `1 sessions, 10 requests, about $1.08 at API list prices, last 14 days
plus subagents: 1 requests, about $0.02

CONTEXT OF A REQUEST  REQUESTS  COST   SHARE
under 100k            7         $0.67  62%
100k to 200k          1         $0.19  17%
200k to 400k          2         $0.22  20%
over 400k             0         $0.00  0%

Context written to the cache again instead of read:
CAUSE                              REQUESTS  TOKENS  EXTRA COST  SHARE
compaction or /clear               1         210k    $0.50       46.8%
idle for over an hour              1         80k     $0.19       17.8%
idle for 5 to 60 minutes           1         55k     $0.13       12.3%
changed within the cache lifetime  0         0       $0.00       0.0%

1-hour cache: too few requests (10, from 200) to tell.

Text added to conversations besides your prompts (every later request re-reads it):
SOURCE                                            TIMES  PER TIME  COST   SHARE
instruction files (CLAUDE.md, rules, memory)      1      15        $0.00  0.0%
skill listing                                     1      10        $0.00  0.0%
hook UserPromptSubmit: hooks/nudge.sh             1      8         $0.00  0.0%
hook PostToolUse (context added, hook not named)  1      7         $0.00  0.0%

1 compactions, from 210k to 40k of context: cost $0.22, saved $0.05 of cache reads, net $-0.17

If conversations had been compacted when they reached:
CONTEXT  SESSIONS AFFECTED  COMPACTIONS  SAVED
100k     1                  2            $-0.15 (-20%)
150k     1                  1            $-0.06 (-8%)
200k     1                  1            $-0.08 (-12%)
250k     0                  0            $0.00 (0%)
300k     0                  0            $0.00 (0%)
400k     0                  0            $0.00 (0%)

Largest sessions:
PROJECT  START       REQUESTS  PEAK  COMPACTIONS  COST
shop     2026-10-10  10        210k  1            $1.08

Dollars are what the tokens would cost at API list prices, cache writes at the 5-minute or 1-hour rate the transcript records; a subscription does not bill per token.

A compaction is assumed to cost one request over the whole context plus a 12k-token summary; later turns grow as they did.

Compacting saves tokens, not detail: the summary forgets what it leaves out. Treat the saving as an upper bound on what is worth giving up.

Sessions are main conversations; what subagents cost is added up apart, since they run in conversations of their own.

Added text is priced as written once to the cache and read by every later request of the session; what a hook printed is named by the command it ran, as the transcript records it.

A cache rewrite is context that the previous request carried but this one did not read from the cache; the cost shown is the extra over a read. The causes are read from the pauses between requests, so "changed within the cache lifetime" means the cached start of the conversation changed while it was warm (an edited instruction file, a changed tool set, another model); a transcript cannot say which.
`,
	"flat": `1 sessions, 10 requests, last 14 days (flat plan: no dollar figures)

CONTEXT OF A REQUEST  REQUESTS  SHARE OF REQUESTS
under 100k            7         70%
100k to 200k          1         10%
200k to 400k          2         20%
over 400k             0         0%

Context written to the cache again instead of read:
CAUSE                              REQUESTS  TOKENS
compaction or /clear               1         210k
idle for over an hour              1         80k
idle for 5 to 60 minutes           1         55k
changed within the cache lifetime  0         0

Text added to conversations besides your prompts (every later request re-reads it):
SOURCE                                            TIMES  PER TIME
instruction files (CLAUDE.md, rules, memory)      1      15
skill listing                                     1      10
hook UserPromptSubmit: hooks/nudge.sh             1      8
hook PostToolUse (context added, hook not named)  1      7

1 compactions, from 210k to 40k of context

Largest sessions:
PROJECT  START       REQUESTS  PEAK  COMPACTIONS
shop     2026-10-10  10        210k  1
`,
}
