# Source files

`cat`, `head` and `tail` of source code (and MCP `compress` with a `file` hint) are handled by
the `cat` optimizer (`fs/code`). Agents edit by exact text, so source is only ever shrunk by
**dropping whole lines**: every line that is shown is byte-exact, indentation and trailing
comments included.

Agents' own file-read tools (Claude Code's Read, for example) are never touched.

## Gaps name what they hide

Each gap becomes one line in the file's own comment syntax, naming the original lines:

```go
// … comments L12-30        a run of comments
	// … L50-89              an elided function body
```

```python
    # … docstring L22-29
```

For `tail` and `sed` output, where absolute line numbers are unknown, the marker says
`N lines` instead. An agent can:

- edit anywhere inside a kept run of lines, since they are exact;
- read a gap back by range: `sed -n 12,30p file` (small, so it passes through untouched);
- read the whole file: `-attr cat_mode=full`, `GASCOIGNE_CAT_MODE=full`, or `gascoigne raw <id>`.

## Modes

| Mode | When (`auto`) | What happens |
|---|---|---|
| `full` | files under `clean_min_tokens` (800) | nothing |
| `clean` | 800–4000 tokens | comment runs dropped |
| `outline` | ≥ `outline_min_tokens` (4000) | clean, plus function bodies elided |

Force a mode for one payload with `-attr cat_mode=<mode>`, for a run with
`GASCOIGNE_CAT_MODE=<mode>`, or for good with `-set optimizers.cat.mode=<mode>`.

### clean

- Comment runs are dropped. A run right above code keeps its first line with words, so a doc
  summary stays (`// Serve accepts incoming connections…`).
- Python docstrings after `def`/`class` keep their summary line.
- Runs cheaper than their marker stay as they are.
- Directives always stay: `//go:` lines, `# type:`, `noqa`, shebangs, coding lines, eslint and
  TypeScript pragmas.
- Lines inside multi-line strings (backticks, triple quotes, heredocs) are never taken for
  comments.

### outline

Clean, plus function bodies longer than `min_body` (4) lines are elided. Signatures and
closing braces stay as they are, so the structure reads like the file:

```python
# outline: 8 bodies elided (cat_mode=full for complete source)
class JSONDecodeError(ValueError):
    """Subclass of ValueError with the following additional properties:
    # … docstring L22-29
    # Note that this exception is used from _json
    def __init__(self, msg, doc, pos):
        # … L32-40

    def __reduce__(self):
        return self.__class__, (self.msg, self.doc, self.pos)
```

Python stays valid as far as it is shown.

## Languages

Each language is one row of the `detect` language table: extensions, comment syntax and how its
function bodies are found. Unknown languages are left alone rather than guessed.

| Outline by | Languages |
|---|---|
| Go parser | Go |
| indentation | Python |
| braces | JavaScript, TypeScript, Java, Kotlin, Scala, Swift, C, C++, C#, Rust, D, PHP, Dart, Zig, Groovy, Solidity, Move, Odin, Perl, R, PowerShell, Vue, Svelte, Astro |
| `end` | Ruby, Crystal, Elixir, Lua, Julia |
| declarations with deeper-indented bodies | Haskell, Elm, OCaml, F#, Nim |
| parenthesis balance | Clojure, Emacs Lisp, Common Lisp, Scheme, Racket |
| comments only | Erlang, SQL, Protobuf, GraphQL, Terraform, YAML, TOML, INI |

The brace outline lexes Rust lifetimes and raw strings, C++ raw strings and digit separators,
nested block comments (Rust, Scala, D) and counts `mod`, `union`, `package` and `extern "C" {` as
containers, so methods and functions inside them are outlined. Shebangs (`ruby`, `node`, `perl`,
`php`, `lua`, `python`, `rdmd`) and some content (Rust) identify a file without a name.

## Savings

| File | Mode | Before | After | Saved |
|---|---|--:|--:|--:|
| Go `net/http/server.go` | outline | 35,098 | 9,363 | 73.3% |
| JavaScript (deep-eql) | outline | 4,166 | 868 | 79.2% |
| Ruby `benchmark.rb` | outline | 6,390 | 1,746 | 72.7% |
| Python `json/decoder.py` | clean | 3,220 | 2,353 | 26.9% |
| Python `json/decoder.py` | outline (forced) | 3,220 | 579 | 82.0% |

Whitespace is never changed to save tokens: runs of spaces cost little with a real tokenizer,
and editing needs them exact. Comments are where source tokens go.
