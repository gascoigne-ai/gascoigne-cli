// Package pcomb builds line matchers from small combinators instead of long
// regular expressions. A rule table written with it reads as data: each
// keyword is its own argument, so it can be commented, reviewed and tested
// one at a time. Matchers are plain string functions that do not allocate,
// and anything they cannot express falls back to Re.
//
// Matchers see one line and answer yes or no. They are anchored at the start
// of the line, like the "^..." patterns they replace.
package pcomb

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Matcher reports whether a line matches.
type Matcher func(line string) bool

// Prefix matches lines that start with any of the given strings.
func Prefix(prefixes ...string) Matcher {
	mustNotBeEmpty("Prefix", prefixes)
	return func(s string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	}
}

// Word matches lines that start with any of the given keywords, as a whole
// word: "Compiling x" matches Word("Compiling"), "Compilingly" does not. It is
// the "(a|b)\b" idiom, so every keyword must end in a word character.
func Word(keywords ...string) Matcher {
	mustNotBeEmpty("Word", keywords)
	for _, k := range keywords {
		if !isWord(k[len(k)-1]) {
			panic("pcomb: Word keyword must end in a letter, digit or _: " + k)
		}
	}
	return func(s string) bool {
		for _, k := range keywords {
			if strings.HasPrefix(s, k) && (len(s) == len(k) || !isWord(s[len(k)])) {
				return true
			}
		}
		return false
	}
}

// Exact matches lines equal to any of the given strings; "" matches a blank line.
func Exact(lines ...string) Matcher {
	if len(lines) == 0 {
		panic("pcomb: Exact needs at least one string")
	}
	return func(s string) bool {
		for _, l := range lines {
			if s == l {
				return true
			}
		}
		return false
	}
}

// Prefixed matches lines that start with prefix followed by something m
// accepts, as in Prefixed("[INFO] ", Prefix("Download", "Scanning")).
func Prefixed(prefix string, m Matcher) Matcher {
	mustNotBeEmpty("Prefixed", []string{prefix})
	return func(s string) bool {
		rest, ok := strings.CutPrefix(s, prefix)
		return ok && m(rest)
	}
}

// Indented matches lines with at least one leading whitespace character
// followed by m. All leading whitespace is skipped before m runs, so m must
// not start with whitespace itself.
func Indented(m Matcher) Matcher {
	return func(s string) bool {
		t := trimSpace(s)
		return len(t) < len(s) && m(t)
	}
}

// Trimmed matches lines where m matches after any leading whitespace.
func Trimmed(m Matcher) Matcher {
	return func(s string) bool { return m(trimSpace(s)) }
}

// Any matches when at least one of ms matches.
func Any(ms ...Matcher) Matcher {
	return func(s string) bool {
		for _, m := range ms {
			if m(s) {
				return true
			}
		}
		return false
	}
}

// Re matches a regular expression anywhere in the line; anchor it with "^"
// when it must start the line. It is the escape hatch for shapes the other
// combinators cannot say, compiled on first use like every lazyre pattern.
func Re(expr string) Matcher {
	return lazyre.New(expr).MatchString
}

func mustNotBeEmpty(fn string, ss []string) {
	if len(ss) == 0 {
		panic("pcomb: " + fn + " needs at least one string")
	}
	for _, s := range ss {
		if s == "" {
			panic("pcomb: " + fn + " got an empty string, which matches every line")
		}
	}
}

// isWord is regexp's \w: ASCII letters, digits and underscore.
func isWord(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// trimSpace drops leading regexp \s: space, \t, \n, \f, \r.
func trimSpace(s string) string {
	i := 0
	for i < len(s) && strings.IndexByte(" \t\n\f\r", s[i]) >= 0 {
		i++
	}
	return s[i:]
}
