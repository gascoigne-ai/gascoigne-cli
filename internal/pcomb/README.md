# pcomb

Line matchers built from small combinators, for rule tables that outlive their author.

A 400-character regex alternation says *what* matches but not *why* each term is there, and a
typo in it fails silently. With `pcomb` each keyword is its own argument, each rule has a name,
and each rule carries lines it must and must not match, checked by `Rules.Verify` in a test.

| Combinator | Matches | Regexp it replaces |
|---|---|---|
| `Prefix("a", "b")` | line starts with any string | `^(a\|b)` |
| `Word("Compiling")` | line starts with the whole word | `^(Compiling)\b` |
| `Exact("a", "b")` | line equals a string | `^(a\|b)$` |
| `Prefixed("[INFO] ", m)` | the prefix, then `m` on the rest of the line | `^\[INFO\] m` |
| `Indented(m)` | one or more leading blanks, then `m` | `^\s+m` |
| `Trimmed(m)` | zero or more leading blanks, then `m` | `^\s*m` |
| `Any(m, n)` | either matches | `m\|n` |
| `Re(expr)` | the regexp, compiled on first use | escape hatch |

Matchers are `func(line string) bool`: no allocation, about 5x faster than the equivalent
`regexp` (`go test ./internal/pcomb -bench .`). `FuzzAgreesWithRegexp` keeps them equal to the
regexps above on arbitrary input. Constructors panic on a misuse that would silently match
every line (an empty string) or never match (`Word` with a keyword not ending in a word character).

## Writing a rule

```go
pcomb.Rules{{
	Name:  "cargo progress",
	Match: pcomb.Indented(pcomb.Word("Compiling", "Checking", "Finished")),
	Yes:   []string{"   Compiling serde v1.0.210"},          // real tool output
	No:    []string{"Compiling serde", "error[E0308]: ..."}, // near misses
}}
```

Use real tool output in `Yes`. Put the near miss that made you add a word boundary or an indent
in `No`, so nobody removes it later. Test the table with `Verify`; see `TestNoiseRulesVerify` in
`internal/compress/diag`.

## Debugging

`Rules.First(line)` returns the rule that matched, by name. Print it when a line is dropped that
should not be.

## Converting a regexp

1. Split the alternation at `|` into one rule per tool behavior, and name each.
2. `^(A|B)\b` is `Word("A", "B")`; `^\s+` in front is `Indented(...)`; a plain `^A` is `Prefix("A")`.
3. Keep `Re` for what the combinators cannot say (digits, repeats, captures). Do not force it.
4. Add `Yes`/`No` examples, run the package tests and `make eval`: savings must not change.

Keep this package free of imports from `optimizer/`, `internal/router` and the other engine
packages; the only dependency is `internal/lazyre` for `Re`.
