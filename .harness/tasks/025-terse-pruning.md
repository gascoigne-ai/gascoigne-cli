status: todo

# compress/terse: drop low-information words from instruction prose

## Goal
Instruction prose (CLAUDE.md, skills) carries articles, fillers and hedges the model does not
need. Whole-word removal cuts tokens; char-level tricks (vowel dropping, abbreviations) split
one-token words and cost more.

## Scope
- Score words by unigram self-information from an embedded frequency list (no model); drop
  the cheapest until a target ratio. Deterministic.
- Never drop negations and modals (not, never, no, must, only), numbers, code spans, fences,
  paths, identifiers, quoted strings, headings, list markers.
- `gascoigne prompts -terse <file>` writes a pruned copy for review (`.bak` kept); no runtime
  use until the A/B holds.

## Acceptance
- Table tests on the never-drop classes; eval case with a CLAUDE.md-shaped fixture.
- `.harness/scripts/ab.sh` pruned vs original instructions: task success must not drop.
