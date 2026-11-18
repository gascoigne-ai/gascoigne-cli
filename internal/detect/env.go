package detect

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var envAssign = lazyre.New(`^[A-Za-z_][A-Za-z0-9_]*=`)

// SplitEnv splits leading VAR=value assignments off a command; values may be
// quoted ("a b", 'x') and stay verbatim.
func SplitEnv(command string) (env, cmd string) {
	command = strings.TrimLeft(command, " \t")
	for envAssign.Regexp().MatchString(command) {
		end := wordEnd(command)
		if end < 0 {
			break
		}
		env += command[:end] + " "
		command = strings.TrimLeft(command[end:], " ")
	}
	return env, command
}

// wordEnd is the index of the first space outside quotes in s, len(s) when
// the word runs to the end, -1 on an unterminated quote.
func wordEnd(text string) int {
	var quote byte
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
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
	return len(text)
}
