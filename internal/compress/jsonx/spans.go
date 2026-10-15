package jsonx

import "encoding/json"

// Spans finds JSON objects and arrays embedded in text: [start, end) byte
// ranges of balanced, valid top-level values, in order. Scanning is bounded
// by a few passes over text, so bracket-heavy prose cannot make it quadratic;
// spans past the budget are not reported.
func Spans(text string) [][2]int {
	var spans [][2]int
	budget := 4*len(text) + 4096
	for i := 0; i < len(text); i++ {
		if text[i] != '{' && text[i] != '[' {
			continue
		}
		end, walked := closing(text, i)
		if budget -= walked; budget < 0 {
			break
		}
		if end > 0 && json.Valid([]byte(text[i:end])) {
			spans = append(spans, [2]int{i, end})
			i = end - 1
		}
	}
	return spans
}

// closing returns the end of the bracket opened at start (0 when unbalanced
// or mismatched) and how many bytes it walked. Brackets in strings are skipped.
func closing(text string, start int) (end, walked int) {
	var stack []byte
	inStr, esc := false, false
	for j := start; j < len(text); j++ {
		c := text[j]
		switch {
		case inStr && esc:
			esc = false
		case inStr && c == '\\':
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{' || c == '[':
			stack = append(stack, c)
		case c == '}' || c == ']':
			open := byte('{')
			if c == ']' {
				open = '['
			}
			if stack[len(stack)-1] != open {
				return 0, j - start + 1
			}
			if stack = stack[:len(stack)-1]; len(stack) == 0 {
				return j + 1, j - start + 1
			}
		}
	}
	return 0, len(text) - start
}
