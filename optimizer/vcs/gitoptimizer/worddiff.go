package gitoptimizer

import "strings"

// wordDiff rewrites blocks of k removed lines followed by k added lines,
// where each pair changes only part of the line, into git's word-diff
// notation: "~prefix[-old-]{+new+}suffix". Both versions stay exact: old is
// prefix+old+suffix, new is prefix+new+suffix. A block is rewritten only when
// every pair qualifies, so lines never shift between pairs.
func wordDiff(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		k := signRun(lines[i:], '-')
		m := signRun(lines[i+k:], '+')
		if k == 0 {
			out = append(out, lines[i])
			i++
			continue
		}
		block, ok := pairs(lines[i:i+k], lines[i+k:i+k+m])
		if !ok {
			block = lines[i : i+k+m]
		}
		out = append(out, block...)
		i += k + m
	}
	return out
}

// signRun counts leading lines starting with sign.
func signRun(lines []string, sign byte) int {
	n := 0
	for n < len(lines) && len(lines[n]) > 0 && lines[n][0] == sign {
		n++
	}
	return n
}

func pairs(removed, added []string) ([]string, bool) {
	if len(removed) != len(added) {
		return nil, false
	}
	out := make([]string, len(removed))
	for i := range removed {
		l, ok := pair(removed[i][1:], added[i][1:])
		if !ok {
			return nil, false
		}
		out[i] = l
	}
	return out, true
}

// pair is one changed line in word-diff form, if that is clearly shorter
// and unambiguous. Edits snap to whitespace so whole words change.
func pair(old, new string) (string, bool) {
	for _, m := range []string{"[-", "-]", "{+", "+}"} {
		if strings.Contains(old, m) || strings.Contains(new, m) {
			return "", false
		}
	}
	p := 0
	for p < len(old) && p < len(new) && old[p] == new[p] {
		p++
	}
	for p > 0 && !isSpace(old[p-1]) {
		p--
	}
	s := 0
	for s < len(old)-p && s < len(new)-p && old[len(old)-1-s] == new[len(new)-1-s] {
		s++
	}
	for s > 0 && !isSpace(old[len(old)-s]) {
		s--
	}
	was, now := old[p:len(old)-s], new[p:len(new)-s]
	if 2*(len(was)+len(now)) > len(new) {
		return "", false // most of the line changed: the two lines read better
	}
	var b strings.Builder
	b.WriteString("~" + old[:p])
	if was != "" {
		b.WriteString("[-" + was + "-]")
	}
	if now != "" {
		b.WriteString("{+" + now + "+}")
	}
	b.WriteString(old[len(old)-s:])
	if b.Len()+12 > len(old)+len(new)+3 {
		return "", false // saves too little to be worth a new notation
	}
	return b.String(), true
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' }
