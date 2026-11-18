package main

import (
	"errors"
	"io"
	"os"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// transcriptTail is how much of a transcript is read for its model.
const transcriptTail = 256 << 10

var modelField = lazyre.New(`"model"\s*:\s*"([a-z][a-z0-9.\-]+[a-z0-9])"`)

// transcriptEnd is the last transcriptTail bytes of a transcript, nil when unreadable.
func transcriptEnd(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := max(0, st.Size()-transcriptTail)
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return nil
	}
	return buf
}

// transcriptModel is the model of the latest request an agent logged in
// its transcript (Claude Code and Gemini CLI give hooks its path), so runs
// are priced with the model that reads them. Only the file's tail is read.
func transcriptModel(path string) string {
	ms := modelField.Regexp().FindAllSubmatch(transcriptEnd(path), -1)
	if len(ms) == 0 {
		return ""
	}
	return string(ms[len(ms)-1][1])
}
