status: later

# LLM proxy: terse note and thinking effort after tool results (experimental)

## Goal
Output tokens cost several times input. Two proxy-side levers: append a short "be terse" note to the
end of the system prompt (cache-safe), and lower thinking effort on turns that only resume after a
tool result. Clamp only: never raise effort.

## Scope
- `receiver/httpproxyreceiver`, Anthropic `/v1/messages` first; off by default behind a setting; a
  held-out share of requests unshaped so the effect is measured, not assumed.
- Ask the user before starting: it changes request semantics.

## Acceptance
- Tests: note placement keeps the cached prefix byte-identical, clamp-only, error turns keep effort.
- A/B (`ab.sh`, harder tasks) shows success unchanged and output tokens down; otherwise move to Tried and dropped.
