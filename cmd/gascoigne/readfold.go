package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// readFoldPointer replaces a Read result identical to the one the agent
	// already holds. Claude numbers the lines of the content itself.
	readFoldPointer = "[gascoigne: same as the Read of this file earlier in this session, unchanged]"

	// readFoldMinBytes: below this the pointer saves nothing worth the risk.
	readFoldMinBytes = 400

	// readFoldMaxAge: Claude Code can drop old tool results by itself, so a
	// read this old is served whole again rather than pointed at.
	readFoldMaxAge = time.Hour
)

// readSeen is what the agent was last shown for one Read: a hash of the text
// and when.
type readSeen struct {
	SHA string    `json:"sha"`
	TS  time.Time `json:"ts"`
}

// readFoldOff: $GASCOIGNE_READ_FOLD=off turns the fold off.
func readFoldOff() bool { return os.Getenv("GASCOIGNE_READ_FOLD") == "off" }

// readStateFile is the per-session record of Reads, in the state dir.
func readStateFile(session string) string {
	return filepath.Join(stateDir(), "reads", filepath.Base(session)+".json")
}

func loadReads(session string) map[string]readSeen {
	seen := map[string]readSeen{}
	if data, err := os.ReadFile(readStateFile(session)); err == nil {
		json.Unmarshal(data, &seen)
	}
	return seen
}

func saveReads(session string, seen map[string]readSeen) error {
	file := readStateFile(session)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// forgetReads clears the record when the context it describes is gone: after
// a compaction, /clear or resume the agent no longer holds the earlier text.
func forgetReads(in hookInput) {
	if in.HookEventName != "SessionStart" || in.SessionID == "" {
		return
	}
	switch in.Source {
	case "compact", "clear", "resume":
		os.Remove(readStateFile(in.SessionID))
	}
}

// foldRead answers a text Read with a pointer when the same file, range and
// text were shown earlier in this session; nil leaves the result alone.
// Subagents have their own context, so they never get a pointer.
func foldRead(in hookInput, resp any) any {
	obj, _ := resp.(map[string]any)
	file, _ := obj["file"].(map[string]any)
	text, _ := file["content"].(string)
	path, _ := in.ToolInput["file_path"].(string)
	if readFoldOff() || obj["type"] != "text" || path == "" || in.SessionID == "" || in.AgentID != "" ||
		len(text) < readFoldMinBytes || strings.HasPrefix(text, "[gascoigne:") {
		return nil
	}
	key := path + "\x00" + readRange(in.ToolInput)
	sum := sha256.Sum256([]byte(text))
	now := time.Now()
	cur := readSeen{SHA: hex.EncodeToString(sum[:]), TS: now}
	seen := loadReads(in.SessionID)
	prev, ok := seen[key]
	if ok && prev.SHA == cur.SHA && now.Sub(prev.TS) < readFoldMaxAge {
		return foldedRead(obj, file)
	}
	seen[key] = cur
	saveReads(in.SessionID, seen)
	return nil
}

// readRange tells apart reads of different parts of one file.
func readRange(input map[string]any) string {
	off, _ := input["offset"].(float64)
	lim, _ := input["limit"].(float64)
	return strconv.Itoa(int(off)) + ":" + strconv.Itoa(int(lim))
}

func foldedRead(obj, file map[string]any) map[string]any {
	nf := maps.Clone(file)
	nf["content"] = readFoldPointer
	no := maps.Clone(obj)
	no["file"] = nf
	return no
}
