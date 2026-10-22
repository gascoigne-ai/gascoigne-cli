package detect

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var envAssign = lazyre.New(`^[A-Za-z_][A-Za-z0-9_]*=`)

// SplitEnv splits leading VAR=value assignments off a command; values may be
// quoted ("a b", 'x') and stay verbatim.
func SplitEnv(s string) (env, cmd string) {
	s = strings.TrimLeft(s, " \t")
	for envAssign.Regexp().MatchString(s) {
		end := wordEnd(s)
		if end < 0 {
			break
		}
		env += s[:end] + " "
		s = strings.TrimLeft(s[end:], " ")
	}
	return env, s
}

// wordEnd is the index of the first space outside quotes in s, len(s) when
// the word runs to the end, -1 on an unterminated quote.
func wordEnd(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case quote == '\'' && c == '\'':
			quote = 0
		case quote == '"':
			if c == '\\' {
				i++
			} else if c == '"' {
				quote = 0
			}
		case quote != 0:
		case c == '\\':
			i++
		case c == '"' || c == '\'':
			quote = c
		case c == ' ':
			return i
		}
	}
	if quote != 0 {
		return -1
	}
	return len(s)
}
