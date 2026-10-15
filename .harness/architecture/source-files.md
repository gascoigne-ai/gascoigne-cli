# Source files (`optimizer/catoptimizer`)

Agents edit by exact text, so source is shrunk only by dropping whole lines. Kept lines are
byte-exact: indentation and trailing comments are untouched, so Python stays valid as far as shown.

**Gap markers.** Each gap is one marker in the file's comment syntax that names the original
lines: `// … comments L12-30`, `# … docstring L3-9`, `// … L50-89` for a body (`N lines` instead
of numbers for `tail`/`sed`). An agent edits within kept runs and reads a gap back by range
(`sed -n 12,30p f`: small, passes untouched) or the whole file (`cat_mode=full`, `gascoigne raw`).

**Modes** (chosen by size):
- **clean** (800–4000 tokens): comment runs go.
  - A run right above code keeps its first line with words (the doc summary). So does a run one
    blank line above code at its indent, unless it comes before any code.
  - Python docstrings after `def`/`class` keep their summary line.
  - Runs cheaper than a marker stay, as do directives (`//go:`, `# type:`, `noqa`, shebang,
    coding, eslint/ts pragmas).
  - Lines inside multi-line strings (backticks, triple quotes, heredocs) are never taken for
    comments.
- **outline** (≥ 4000 tokens): clean, plus function bodies elided; signatures and closing braces
  kept as they are.

**Scope.**
- Language-agnostic: one `detect.Language` row per language (extensions, outline class, comment
  syntax, lexical quirks), else by class (`detect.Lang`). Unknown languages are left alone.
- Outline style per row: braces (c-like), `end` blocks, declaration + indented body, parenthesis
  balance, Go AST, Python indentation; rows without one only lose comments.
- The brace outline lexes per row: nested block comments (Rust, Scala, D `/+ +/`), raw strings
  (Rust `r#"…"#`, C++ `R"x(…)x"`), Rust lifetimes vs char literals, C++ digit separators; `mod`,
  `union`, `package` and `extern "C" {` open containers, so their functions are outlined.
- Only `cat`/`head`/`tail` output and MCP `compress` are touched, never an agent's own
  file-read tool.
