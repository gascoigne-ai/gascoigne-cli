package pricing

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SplitArgs splits a command line into arguments: spaces separate, single and double quotes
// group, a backslash escapes the next character. There is no shell: no expansion, pipes or
// redirects.
func SplitArgs(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	started := false
	var quote rune
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, started = r, true
		case r == ' ' || r == '\t':
			if started {
				args, started = append(args, current.String()), false
				current.Reset()
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("command %s: unterminated quote", strconv.Quote(line))
	}
	if started {
		args = append(args, current.String())
	}
	if len(args) == 0 {
		return nil, errors.New("command is empty")
	}
	return args, nil
}
