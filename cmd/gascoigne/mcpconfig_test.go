package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProxiedConfigs(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(t.TempDir(), "shop")
	gh := []string{"npx", "-y", "github-mcp"}
	pg := []string{"npx", "-y", "pg-mcp", "postgres://u:secret@h/prod"}
	writeFile(t, filepath.Join(home, ".claude.json"), `{
	  "mcpServers": {
	    "github": {"command": "/usr/bin/gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "github-mcp"]},
	    "plain": {"command": "npx", "args": ["-y", "not-proxied"]},
	    "remote": {"type": "http", "url": "https://x"},
	    "web": {"command": "gascoigne", "args": ["mcp-proxy", "-url", "https://mcp.test/mcp?key=secret", "-header", "Authorization: Bearer secret"]}
	  },
	  "projects": {"`+proj+`": {"mcpServers": {"db": {"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "pg-mcp", "postgres://u:secret@h/prod"]}}}}
	}`)
	writeFile(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers": {"gh2": {"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "github-mcp"]}}}`)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"cur": {"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "github-mcp"]}}}`)
	writeFile(t, filepath.Join(home, "broken", ".mcp.json"), `{not json`)

	got := proxiedConfigs(home, []string{proj, filepath.Join(home, "broken"), proj, filepath.Join(home, "missing")})

	byKey := func(entries []mcpConfigEntry) map[string]mcpConfigEntry {
		m := map[string]mcpConfigEntry{}
		for _, e := range entries {
			m[e.Key] = e
		}
		return m
	}
	g := byKey(got[mcpproxyreceiver.CommandHash(gh)])
	if len(g) != 3 {
		t.Fatalf("one server configured in three places: %+v", g)
	}
	if e := g["github"]; e.Agent != "claude" || e.Scope != "user" || e.Remove != "claude mcp remove github --scope user" {
		t.Errorf("user entry: %+v", e)
	}
	if e := g["gh2"]; e.Scope != "project" || e.File != filepath.Join(proj, ".mcp.json") || !strings.HasSuffix(e.Remove, "claude mcp remove gh2 --scope project") || !strings.HasPrefix(e.Remove, "cd ") {
		t.Errorf("project entry: %+v", e)
	}
	if e := g["cur"]; e.Agent != "cursor" || !strings.HasPrefix(e.Remove, "delete the cur entry from ") {
		t.Errorf("cursor entry: %+v", e)
	}
	d := got[mcpproxyreceiver.CommandHash(pg)]
	if len(d) != 1 || d[0].Scope != "local" || d[0].Key != "db" || !strings.Contains(d[0].Remove, "--scope local") {
		t.Errorf("local entry: %+v", d)
	}
	if w := got[mcpproxyreceiver.CommandHash([]string{"https://mcp.test/mcp?key=secret"})]; len(w) != 1 || w[0].Key != "web" {
		t.Errorf("remote entry: %+v", w)
	}
	if _, ok := got[mcpproxyreceiver.CommandHash([]string{"npx", "-y", "not-proxied"})]; ok {
		t.Error("a server that does not go through the proxy was found")
	}
	if len(got) != 3 {
		t.Errorf("hashes found: %d", len(got))
	}
	for _, entries := range got {
		for _, e := range entries {
			if strings.Contains(e.Remove, "secret") {
				t.Errorf("the command line leaks into %q", e.Remove)
			}
		}
	}
}

func TestReviewShowsWhereAServerIsConfigured(t *testing.T) {
	home := sandbox(t)
	writeFile(t, filepath.Join(home, ".claude.json"), `{"mcpServers": {"github": {"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "github-mcp"]}}}`)
	id := "github-mcp-server#" + mcpproxyreceiver.CommandHash([]string{"npx", "-y", "github-mcp"})
	l, h := testDashboard(t)
	if err := l.RecordToolList(t.Context(), "claude", id, map[string]int{"search": 300}, nil); err != nil {
		t.Fatal(err)
	}
	if err := l.RecordToolList(t.Context(), "claude", "other#00000000", map[string]int{"x": 100}, nil); err != nil {
		t.Fatal(err)
	}
	review, err := loadReview(t.Context(), l, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range review.Servers {
		switch s.ID {
		case id:
			if len(s.Config) != 1 || s.Config[0].Key != "github" {
				t.Errorf("config of the known server: %+v", s.Config)
			}
		default:
			if s.Config == nil || len(s.Config) != 0 {
				t.Errorf("an unknown server must have an empty list, not %#v", s.Config)
			}
		}
	}
	w := get(h, "GET", "/api/mcp")
	if !strings.Contains(w.Body.String(), "claude mcp remove github --scope user") || strings.Contains(w.Body.String(), "null") {
		t.Fatalf("api: %s", w.Body.String())
	}
}
