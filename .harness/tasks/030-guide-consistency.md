status: todo

# Usage guide: match each agent's real capabilities

## Goal
The guide (`cmd/gascoigne/guide.go`) tells agents things they cannot do. Codex gets the
command-rewrite advice from `install` (AGENTS.md block uses `guide(true)`) although codex does not
rewrite commands; pi's guide names the fetch, show and retrieve tools although pi has no MCP.

## Scope
- `guide(rewrites, mcp bool)`: without `mcp`, drop the fetch/compress line, the "(or the show
  tool)" and "or call retrieve" parts; keep the CLI forms (`gascoigne show`, `gascoigne raw`).
- codex `install` (AGENTS.md block) uses `guide(false, true)`, same as `wrap`; pi uses
  `guide(true, false)`. Update the other callers (`opencode.go`, cursor rule, claude SessionStart).
- Codex and pi share the project `AGENTS.md` block: the later install wins; say so in a comment.
- Gemini's AfterTool covers shell and `mcp_*` only: say it in the architecture Agents section.

## Acceptance
- Table test over agents: the guide text has no tool name the agent lacks, and rewrite advice only
  where commands are rewritten.
- `.harness/architecture.md` §Agents "Usage guide" updated in the same commit.
