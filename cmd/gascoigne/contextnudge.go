package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// contextNudgeTokens is the context size above which a prompt gets a note:
// requests above 400k tokens were 42% of cost, and a compaction pays back
// well before that (see `gascoigne context`).
const contextNudgeTokens = 250_000

// contextNudgeLimit is the threshold: $GASCOIGNE_CONTEXT_NUDGE in tokens,
// 0 or "off" for never.
func contextNudgeLimit() int {
	v := os.Getenv("GASCOIGNE_CONTEXT_NUDGE")
	if v == "" {
		return contextNudgeTokens
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// contextNudge is the note a prompt gets when the conversation's last request
// read more than the threshold. It only tells; the user and agent decide.
func contextNudge(transcript string) string {
	limit := contextNudgeLimit()
	if limit == 0 {
		return ""
	}
	n := lastContext(transcriptEnd(transcript))
	if n < limit {
		return ""
	}
	return fmt.Sprintf("gascoigne: context is %dk tokens: finish or /compact", n/1000)
}

// lastContext is the input size (input + cache read + cache write) of the
// latest main-conversation request in a transcript tail, 0 when there is none.
// The first line of a tail may be cut; it fails to parse and is skipped.
func lastContext(tail []byte) int {
	lines := bytes.Split(tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var t usageLine
		if json.Unmarshal(lines[i], &t) != nil || t.Type != "assistant" || t.IsSidechain || strings.HasPrefix(t.Message.Model, "<") {
			continue
		}
		u := t.Message.Usage
		if n := u.Input + u.CacheRead + u.CacheCreate; n > 0 {
			return n
		}
	}
	return 0
}
