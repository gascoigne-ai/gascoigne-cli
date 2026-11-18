package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
)

// cmdReplay rebuilds the requests of recorded agent sessions (Claude Code
// transcripts) and sends them through the LLM API proxy against a stand-in
// upstream: tool-result savings on real sessions, and whether every message
// an earlier request sent is sent byte-identical again (the prompt cache
// depends on it). Nothing leaves the machine; a throwaway ledger is used.
func cmdReplay(ctx context.Context, args []string) error {
	var (
		dir      string
		sessions int
		asJSON   bool
		against  string
		sets     listFlag
	)
	fs := newFlags("replay")
	fs.StringVar(&dir, "dir", filepath.Join(home(), ".claude", "projects"), "`directory` with session transcripts (*.jsonl), searched recursively")
	fs.IntVar(&sessions, "sessions", 10, "newest `n` transcripts to replay")
	fs.BoolVar(&asJSON, "json", false, "print JSON instead of a table")
	fs.Var(&sets, "set", "config override `path=yaml` (repeatable, after $GASCOIGNE_SET), to compare settings on the same transcripts")
	fs.StringVar(&against, "compare", "", "also print what changed since an earlier `report` (replay -json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Transcripts named as arguments, so two builds replay the same ones.
	files := fs.Args()
	if len(files) == 0 {
		var err error
		if files, err = transcripts(dir, sessions); err != nil {
			return err
		}
	}
	old, err := readReplayReport(against)
	if err != nil {
		return err
	}
	rep, err := replayAll(ctx, files, sets)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(rep)
	}
	rep.print()
	if old != nil {
		fmt.Println()
		rep.compare(old)
	}
	return nil
}

// replayAll replays the transcripts through the LLM proxy against a stand-in
// upstream and a throwaway ledger, with the config overrides in sets.
func replayAll(ctx context.Context, files []string, sets listFlag) (replayReport, error) {
	rep := replayReport{Tools: map[string]*toolReplay{}}
	tmp, err := os.MkdirTemp("", "gascoigne-replay-")
	if err != nil {
		return rep, err
	}
	defer os.RemoveAll(tmp)
	ledgerPath := filepath.Join(tmp, "ledger.db")
	os.Setenv(ledger.EnvPath, ledgerPath)

	up := &upstream{}
	srv := httptest.NewServer(up)
	defer srv.Close()
	var mu sync.Mutex
	proxy, closeProxy, err := llmProxyWith(ctx, &mu, ledgerPath, srv.URL, slog.New(slog.DiscardHandler),
		config.Overrides{Set: append(splitList(os.Getenv("GASCOIGNE_SET"), ";\n"), sets...)})
	if err != nil {
		return rep, err
	}
	defer closeProxy()

	for _, file := range files {
		if err := replayFile(ctx, proxy, up, file, &rep); err != nil {
			return rep, fmt.Errorf("%s: %w", file, err)
		}
		rep.Sessions++
	}
	return rep, nil
}

// transcripts returns the newest n *.jsonl files under dir.
func transcripts(dir string, limit int) ([]string, error) {
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, file{path, info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(left, right file) int { return right.mod.Compare(left.mod) })
	var out []string
	for _, f := range files[:min(limit, len(files))] {
		out = append(out, f.path)
	}
	return out, nil
}

// upstream stands in for the API: it keeps the body it was sent.
type upstream struct{ body []byte }

func (u *upstream) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	u.body, _ = io.ReadAll(request.Body)
	writer.Header().Set("Content-Type", "application/json")
	io.WriteString(writer, `{"type":"message","content":[]}`)
}

// replayFile sends one request per model turn that follows tool results.
func replayFile(ctx context.Context, proxy http.Handler, up *upstream, path string, rep *replayReport) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sess := &session{tools: map[string]string{}, results: map[string]resultTokens{}, tokens: map[string]int{}}
	lastID := ""
	var addErr error
	scanErr := scanTranscript(file, func(raw []byte) {
		var line transcriptLine
		if addErr != nil || json.Unmarshal(raw, &line) != nil || line.Message == nil || line.IsSidechain || (line.Type != "user" && line.Type != "assistant") {
			return
		}
		addErr = sess.add(ctx, proxy, up, path, rep, line, &lastID)
	})
	if addErr != nil {
		return addErr
	}
	if scanErr != nil {
		return scanErr
	}
	rep.addResults(sess)
	return nil
}
