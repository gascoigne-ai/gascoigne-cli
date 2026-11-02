package logoptimizer

import "strings"

// categoricalKeys name fields whose number picks a case, not an amount: a
// 503 among 200s is the line to read, so these values stay in the repeat key.
var categoricalKeys = map[string]bool{
	"status": true, "status_code": true, "statuscode": true, "http_status": true, "http_code": true,
	"code": true, "exit": true, "exit_code": true, "exitcode": true, "exit status": true, "exit code": true,
	"rc": true, "retcode": true, "return_code": true, "errno": true, "signal": true,
}

// protectCodes rewrites the digits of categorical values ("status=503",
// "exit code 137": a key, one of "=: ", an optional space, up to three
// digits) as capital letters, which the masking in compress.Normalize leaves
// alone, so lines with different codes get different keys. Only the key sees
// the result. A scan, not a regexp: this runs on every line.
func protectCodes(line string) string {
	var out []byte
	for i := 0; i < len(line); i++ {
		if !isDigit(line[i]) || i > 0 && isWord(line[i-1]) {
			continue
		}
		j := i
		for j < len(line) && isDigit(line[j]) {
			j++
		}
		if j-i <= 3 && (j == len(line) || !isWord(line[j])) && categoricalBefore(line, i) {
			if out == nil {
				out = []byte(line)
			}
			for k := i; k < j; k++ {
				out[k] = 'A' + line[k] - '0'
			}
		}
		i = j
	}
	if out == nil {
		return line
	}
	return string(out)
}

// categoricalBefore reports whether a categorical key, a separator and at
// most one space end right before line[i].
func categoricalBefore(line string, i int) bool {
	for _, sep := range []int{i - 1, i - 2} {
		if sep < 0 || !strings.ContainsRune("=: ", rune(line[sep])) {
			continue
		}
		if sep == i-2 && line[i-1] != ' ' && line[i-1] != '\t' {
			continue
		}
		if isCategoricalKey(line[:sep]) {
			return true
		}
	}
	return false
}

// isCategoricalKey reports whether the text ends with a categorical key as a
// whole word (or two: "exit code").
func isCategoricalKey(s string) bool {
	start := len(s)
	for start > 0 && isWord(s[start-1]) {
		start--
	}
	if categoricalKeys[strings.ToLower(s[start:])] {
		return true
	}
	if start == 0 || s[start-1] != ' ' {
		return false
	}
	prev := start - 1
	for prev > 0 && isWord(s[prev-1]) {
		prev--
	}
	return categoricalKeys[strings.ToLower(s[prev:start-1]+" "+s[start:])]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWord(c byte) bool {
	return isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}
