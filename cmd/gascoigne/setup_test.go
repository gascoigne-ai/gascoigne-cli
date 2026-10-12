package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func postSetup(h http.Handler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/setup", strings.NewReader(body))
	r.Host = "127.0.0.1:7878"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPendingDecisions(t *testing.T) {
	sandbox(t) // no agents on PATH: only the two own questions
	if got := pendingDecisions(prefs{}); len(got) != 2 {
		t.Fatalf("a fresh install has both questions open: %+v", got)
	}
	if got := pendingDecisions(prefs{Allow: "none", Autosync: "off"}); len(got) != 0 {
		t.Fatalf("answered, even with the least change: %+v", got)
	}
	got := pendingDecisions(prefs{Allow: "local"})
	if len(got) != 1 || got[0].ID != "autosync" {
		t.Fatalf("only autosync is open: %+v", got)
	}
}

func TestApplyAnswers(t *testing.T) {
	sandbox(t)
	for _, bad := range []map[string]string{
		{"allow": "everything"},
		{"fetch": "yes"},
		{"allow": "local", "autosync": "maybe"},
	} {
		if _, err := applyAnswers(bad); err == nil {
			t.Errorf("%v must be refused", bad)
		}
	}
	if p := readPrefs(); p.Allow != "" || p.Autosync != "" {
		t.Fatalf("a refused request saved something: %+v", p)
	}
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if err := recordInstall("claude", false, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	lines, err := applyAnswers(map[string]string{"allow": "local", "autosync": "on"})
	if err != nil {
		t.Fatal(err)
	}
	if p := readPrefs(); p.Allow != "local" || p.Autosync != "on" {
		t.Fatalf("prefs: %+v", p)
	}
	home, _ := os.UserHomeDir()
	if got := read(t, home+"/.claude/settings.json"); !strings.Contains(got, "mcp__gascoigne__retrieve") {
		t.Errorf("user settings lack the allow rules:\n%s", got)
	}
	if _, err := os.Stat(".claude/settings.json"); !os.IsNotExist(err) {
		t.Error("the page must not write project files")
	}
	if got := strings.Join(lines, "\n"); !strings.Contains(got, "claude: updated") || !strings.Contains(got, "run gascoigne install there") {
		t.Errorf("report:\n%s", got)
	}
}

func TestSetupTokens(t *testing.T) {
	sandbox(t)
	if validSetupToken("") || validSetupToken("anything") {
		t.Fatal("no token saved: nothing is valid")
	}
	tok, err := newSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if !validSetupToken(tok) || validSetupToken(tok+"x") || validSetupToken("") {
		t.Fatal("only the saved token is valid")
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(setupTokenFile(), old, old); err != nil {
		t.Fatal(err)
	}
	if !validSetupToken(tok) {
		t.Fatal("file times do not matter, the saved expiry does")
	}
	if err := os.WriteFile(setupTokenFile(), []byte(`{"token":"`+tok+`","expires":"2000-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if validSetupToken(tok) {
		t.Fatal("an expired token must not work")
	}
}

func TestSetupPage(t *testing.T) {
	sandbox(t)
	_, h := testDashboard(t)
	if w := get(h, "GET", "/setup"); w.Code != 200 || !strings.Contains(w.Body.String(), "gascoigne setup") {
		t.Fatalf("page: %d", w.Code)
	}
	if w := get(h, "GET", "/api/setup"); w.Code != 403 {
		t.Fatalf("no token: %d", w.Code)
	}
	tok, err := newSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/setup?token="+tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"allow"`) {
		t.Fatalf("with token: %d %s", w.Code, w.Body.String())
	}
	if w := postSetup(h, `{"token":"nope","answers":{"allow":"all"}}`); w.Code != 403 {
		t.Fatalf("wrong token: %d", w.Code)
	}
	if w := postSetup(h, `{"token":"`+tok+`","answers":{"allow":"root"}}`); w.Code != 400 {
		t.Fatalf("bad answer: %d", w.Code)
	}
	if p := readPrefs(); p.Allow != "" {
		t.Fatalf("a refused request saved %+v", p)
	}
	if w := postSetup(h, `{"token":"`+tok+`","answers":{"allow":"none","autosync":"off"}}`); w.Code != 200 {
		t.Fatalf("valid: %d %s", w.Code, w.Body.String())
	}
	if p := readPrefs(); p.Allow != "none" || p.Autosync != "off" {
		t.Fatalf("prefs: %+v", p)
	}
	if w := postSetup(h, `{"token":"`+tok+`","answers":{"allow":"all"}}`); w.Code != 403 {
		t.Fatalf("a link answers once: %d", w.Code)
	}
	cross := httptest.NewRequest("POST", "/api/setup", strings.NewReader("{}"))
	cross.Host = "127.0.0.1:7878"
	cross.Header.Set("Origin", "http://evil.example")
	if w := httptest.NewRecorder(); func() int { h.ServeHTTP(w, cross); return w.Code }() != 403 {
		t.Error("cross-origin writes are refused")
	}
}

func TestAskTerminal(t *testing.T) {
	sandbox(t)
	ds := pendingDecisions(prefs{})
	var out strings.Builder
	got, err := ask(strings.NewReader("\n2\n"), &out, ds, false)
	if err != nil {
		t.Fatal(err)
	}
	if got["allow"] != "local" || got["autosync"] != "off" {
		t.Fatalf("empty = default, 2 = second option: %v", got)
	}
	got, err = ask(strings.NewReader("9\nx\n2\n\n"), &out, ds[:1], false)
	if err != nil || got["allow"] != "all" {
		t.Fatalf("bad input asks again: %v %v", got, err)
	}
	if got, _ := ask(strings.NewReader(""), &out, ds, true); got["allow"] != "local" || got["autosync"] != "on" {
		t.Fatalf("-y takes defaults without reading: %v", got)
	}
	if _, err := ask(strings.NewReader(""), &out, ds, false); err == nil {
		t.Error("end of input without an answer is an error")
	}
}

func TestSetupNudgeComesBackUntilAnswered(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off") // never start a server here
	if setupNudge("startup") != "" {
		t.Fatal("no installs: nothing to ask about")
	}
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setupNudge("startup"), "gascoigne setup") {
		t.Fatal("open questions after an install are announced")
	}
	if setupNudge("startup") != "" {
		t.Fatal("quick restarts do not open a window each time")
	}
	// The page was closed without an answer: the next session asks again.
	p := readPrefs()
	p.AskedAt = time.Now().Add(-askInterval - time.Minute)
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	if setupNudge("compact") != "" || setupNudge("clear") != "" {
		t.Fatal("a compaction or /clear is not a new session")
	}
	if !strings.Contains(setupNudge("resume"), "gascoigne setup") {
		t.Fatal("not answered: the page comes back with the next session")
	}
	// Answered: it stays away.
	if _, err := applyAnswers(map[string]string{"allow": "none", "autosync": "off"}); err != nil {
		t.Fatal(err)
	}
	p = readPrefs()
	p.AskedAt = time.Now().Add(-24 * time.Hour)
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	if got := setupNudge("startup"); got != "" {
		t.Fatalf("everything answered, got %q", got)
	}
}

// fakeAgent puts an executable named like an agent on PATH, so it counts as
// found.
func fakeAgent(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestNewAgentIsOnlySignaled(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	fakeAgent(t, "codex")
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	p := readPrefs()
	if got := newAgents(p); len(got) != 1 || got[0] != "codex" {
		t.Fatalf("new agents: %v", got)
	}
	var q *decision
	for _, d := range pendingDecisions(p) {
		if d.ID == "integrate:codex" {
			q = &d
		}
	}
	if q == nil || q.Default != "yes" {
		t.Fatalf("a question for codex is open: %+v", pendingDecisions(p))
	}

	// Signaled once: the session context says so, nothing is installed.
	note := setupNudge("startup")
	if !strings.Contains(note, "codex") || !strings.Contains(note, "installs nothing by itself") {
		t.Fatalf("note: %q", note)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".codex/config.toml")); !os.IsNotExist(err) {
		t.Fatal("found an agent must not install it")
	}
	if got := readInstalls(); len(got) != 1 {
		t.Fatalf("no install recorded for codex: %+v", got)
	}
	if again := setupNudge("startup"); again != "" {
		t.Errorf("within the interval it is not announced again: %q", again)
	}
}

func TestNewAgentAnswers(t *testing.T) {
	home := sandbox(t)
	fakeAgent(t, "codex")
	if _, err := applyAnswers(map[string]string{"integrate:gemini": "yes"}); err == nil {
		t.Fatal("only agents on PATH can be asked about")
	}
	if _, err := applyAnswers(map[string]string{"integrate:codex": "no"}); err != nil {
		t.Fatal(err)
	}
	if got := readPrefs().Declined; len(got) != 1 || got[0] != "codex" {
		t.Fatalf("declined: %v", got)
	}
	if got := newAgents(readPrefs()); len(got) != 1 {
		t.Fatal("a declined agent still shows (as answered no), the question is not pending")
	}
	for _, d := range pendingDecisions(readPrefs()) {
		if d.ID == "integrate:codex" {
			t.Error("a declined agent is not asked again")
		}
	}
	lines, err := applyAnswers(map[string]string{"integrate:codex": "yes"}) // changed mind: setup -all
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(lines, "\n"); !strings.Contains(got, "codex: set up") {
		t.Fatalf("report: %s", got)
	}
	if len(readPrefs().Declined) != 0 {
		t.Error("yes clears the decline")
	}
	if got := read(t, filepath.Join(home, ".codex/config.toml")); !strings.Contains(got, "gascoigne") {
		t.Errorf("codex config:\n%s", got)
	}
	if got := readInstalls(); len(got) != 1 || got[0].Agent != "codex" || !got[0].Global {
		t.Errorf("installs: %+v", got)
	}
}

func TestSetupAsksWhereItCan(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("CI", "")
	t.Setenv("GASCOIGNE_SERVER", "")
	if runtime.GOOS == "linux" && !useBrowser(false, false) {
		t.Fatal("a display and no CI: the browser")
	}
	for name, set := range map[string]func(){
		"defaults asked":   func() {},
		"terminal chosen":  func() {},
		"CI":               func() { t.Setenv("CI", "true") },
		"server off":       func() { t.Setenv("GASCOIGNE_SERVER", "off") },
		"no display (ssh)": func() { t.Setenv("DISPLAY", "") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DISPLAY", ":0")
			t.Setenv("CI", "")
			t.Setenv("GASCOIGNE_SERVER", "")
			set()
			yes, term := name == "defaults asked", name == "terminal chosen"
			if useBrowser(yes, term) && !(runtime.GOOS == "darwin" || runtime.GOOS == "windows") || useBrowser(yes, term) && name != "no display (ssh)" {
				t.Errorf("%s: must ask in the terminal", name)
			}
		})
	}
	if hasDisplay("linux") != (os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "") || !hasDisplay("darwin") || !hasDisplay("windows") {
		t.Error("hasDisplay")
	}
}

func TestSetupViewTellsWhatWasFound(t *testing.T) {
	sandbox(t)
	fakeAgent(t, "codex")
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	list := readInstalls()
	list[0].Registration = registrationVersion - 1
	if err := writeInstalls(list); err != nil {
		t.Fatal(err)
	}
	v := buildSetupView(prefs{Declined: []string{"codex"}})
	states := map[string]string{}
	for _, a := range v.Agents {
		states[a.Name+"/"+a.Scope] = a.State
	}
	if states["claude/all projects"] != "older" || states["codex/"] != "declined" || len(v.Agents) != 2 {
		t.Errorf("agents: %+v", v.Agents)
	}
	if v.Version == "" {
		t.Error("the page shows the version")
	}
	var ids []string
	for _, q := range v.Questions {
		ids = append(ids, q.Group+":"+q.ID)
		if len(q.Options) < 2 || q.Text == "" {
			t.Errorf("%s needs a sentence and options: %+v", q.ID, q)
		}
		for _, o := range q.Options {
			if o.Detail == "" && !q.Optional {
				t.Errorf("%s/%s: say what choosing it does", q.ID, o.Value)
			}
		}
	}
	// The immediate update comes before the standing one; groups drive the sections.
	if got := strings.Join(ids, " "); !strings.HasPrefix(got, "permissions:allow updates:update updates:autosync agents:integrate:codex experimental:experimental:") {
		t.Errorf("questions: %s", got)
	}
}

func TestExperimentalSetup(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_EXPERIMENTAL", "")

	// Experimental features never count as open questions.
	for _, d := range pendingDecisions(prefs{}) {
		if strings.HasPrefix(d.ID, experimentPrefix) {
			t.Fatalf("an optional feature is not a pending question: %+v", d)
		}
	}
	if experimentalOn("learn") {
		t.Fatal("off until turned on")
	}
	lines, err := applyAnswers(map[string]string{"experimental:learn": "on", "experimental:continuity": "on"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Errorf("no installs to rewrite for a feature switch: %v", lines)
	}
	if got := readPrefs().Experimental; len(got) != 2 || got[0] != "continuity" || got[1] != "learn" {
		t.Fatalf("saved: %v", got)
	}
	if !experimentalOn("learn") || !experimentalOn("continuity") || experimentalOn("prompts") {
		t.Fatal("the saved choice turns the feature on")
	}
	if !available(command{name: "learn", experimental: true}) || available(command{name: "prompts", experimental: true}) {
		t.Error("an experimental command is available once turned on, not before")
	}
	for _, d := range decisions(readPrefs()) {
		if d.ID == "experimental:learn" && d.Current != "on" {
			t.Errorf("the page shows the saved answer: %+v", d)
		}
	}
	if _, err := applyAnswers(map[string]string{"experimental:learn": "off"}); err != nil {
		t.Fatal(err)
	}
	if experimentalOn("learn") || len(readPrefs().Experimental) != 1 {
		t.Errorf("off turns it off again: %v", readPrefs().Experimental)
	}
	if _, err := applyAnswers(map[string]string{"experimental:nonsense": "on"}); err == nil {
		t.Error("only listed features can be turned on")
	}
	if _, err := applyAnswers(map[string]string{"experimental:learn": "maybe"}); err == nil {
		t.Error("only on or off")
	}
}

func TestEveryExperimentalCommandCanBeTurnedOnInSetup(t *testing.T) {
	listed := map[string]bool{}
	for _, x := range experiments {
		listed[x.name] = true
	}
	for _, c := range commandList {
		if c.experimental && !listed[c.name] {
			t.Errorf("experimental command %q has no question in setup (experiments in setup.go)", c.name)
		}
	}
}

func TestExperimentsExplainThemselves(t *testing.T) {
	ds := experimentDecisions(prefs{})
	if len(ds) != len(experiments) || len(ds) == 0 {
		t.Fatalf("one question per feature: %d", len(ds))
	}
	for _, d := range ds {
		if !d.Optional || d.Group != "experimental" {
			t.Errorf("%s: optional, in the experimental section: %+v", d.ID, d)
		}
		for name, text := range map[string]string{"text": d.Text, "example": d.Example, "how it works": d.How, "impact": d.Impact} {
			if len(strings.TrimSpace(text)) < 20 {
				t.Errorf("%s: say more under %q: %q", d.ID, name, text)
			}
		}
	}
	// The terminal prints the three parts before asking.
	var out strings.Builder
	if _, err := ask(strings.NewReader("\n\n\n"), &out, ds, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Example:", "How it works:", "Impact:", "gascoigne: your state before"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("terminal question lacks %q", want)
		}
	}
}
