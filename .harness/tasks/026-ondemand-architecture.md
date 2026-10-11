status: todo

# Load architecture.md on demand

## Goal
`gascoigne prompts` here: 18.8k claude tokens of instructions per request, 14k of it
`.harness/architecture.md` imported by CLAUDE.md.

## Scope
- Needs user approval (changes agent guidance). Replace the `@.harness/architecture.md` import
  with a short index: which section to read when touching which package.
- Dedupe what CLAUDE.md, architecture.md and code-style.md say twice.

## Acceptance
- `gascoigne prompts` total before/after in the commit message; ab.sh task success holds.
