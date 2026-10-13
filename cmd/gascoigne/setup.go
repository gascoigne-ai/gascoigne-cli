package main

import (
	"bufio"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/prose"
	"github.com/gascoigne-ai/gascoigne-cli/version"
)

//go:embed setup.html
var setupHTML []byte

// A decision is one question gascoigne asks the user before it changes
// something beyond its own files: in the browser (/setup) or in the terminal
// (gascoigne setup). Nothing it covers is done until the user answers.
type decision struct {
	ID      string   `json:"id"`
	Group   string   `json:"group"` // permissions, updates or agents: the page's sections
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Options []option `json:"options"`
	Default string   `json:"default"`
	Current string   `json:"current"` // the saved answer, "" = not decided
	// Optional questions always have a current answer and never count as
	// open: turning an experimental feature on is up to the user.
	Optional bool `json:"optional,omitempty"`
	// Example, How and Impact explain a feature before it is turned on: what
	// it looks like, how it works, what it changes for the user.
	Example string `json:"example,omitempty"`
	How     string `json:"how,omitempty"`
	Impact  string `json:"impact,omitempty"`
}

type option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"` // what choosing it does, in a sentence
}

// decisions are the questions, with the saved answers filled in.
func decisions(p prefs) []decision {
	ds := []decision{
		{ID: "allow", Group: "permissions", Current: p.Allow, Default: "local",
			Title: "Fewer approval prompts",
			Text:  "Claude asks before each use of gascoigne's own tools. Allowing them adds rules to claude's settings; gascoigne unwrap removes exactly those.",
			Options: []option{
				{"local", "Allow the read-only tools", "retrieve, show, stats and compress only read what gascoigne already holds."},
				{"all", "Also allow fetch", "fetch reads web pages and APIs, so it reaches the network."},
				{"none", "Keep asking", "claude asks before every gascoigne tool call, as now."},
			}},
		{ID: "autosync", Group: "updates", Current: p.Autosync, Default: "on",
			Title: "Update hooks automatically after upgrades",
			Text:  "A new gascoigne can add or change the hooks it installed in your agents.",
			Options: []option{
				{"on", "Keep them up to date", "At the next session start after an upgrade, in your user-level configs. Other keys stay and a backup is kept. Project files are never touched automatically."},
				{"off", "Only when I ask", "Nothing changes until you run gascoigne upgrade."},
			}},
	}
	if q, ok := updateDecision(p); ok {
		ds = slices.Insert(ds, 1, q) // the immediate update before the standing one
	}
	ds = append(ds, releaseDecision(p), mcpReviewDecision(p))
	for _, name := range newAgents(p) {
		ds = append(ds, decision{ID: integratePrefix + name, Default: "yes", Current: declinedAs(p, name),
			Group: "agents",
			Title: "Use gascoigne with " + name,
			Text:  name + " is on this machine, but gascoigne is not connected to it, so its command output is not shortened.",
			Options: []option{
				{"yes", "Set it up for " + name, "Writes gascoigne's hooks and MCP server into your user-level " + name + " config. A backup is kept; gascoigne unwrap removes them."},
				{"no", "Not for " + name, "Nothing is installed, and you are not asked again."},
			}})
	}
	return append(ds, experimentDecisions(p)...)
}

// staleInstalls are the recorded installs written by an older registration.
func staleInstalls() []install {
	return slices.DeleteFunc(readInstalls(), func(e install) bool { return e.Registration >= registrationVersion })
}

// updateDecision asks whether to bring older installs up to this version,
// unless autosync does it by itself.
func updateDecision(p prefs) (decision, bool) {
	stale := staleInstalls()
	if len(stale) == 0 || p.Autosync == "on" {
		return decision{}, false
	}
	var which, news []string
	oldest := registrationVersion
	for _, e := range stale {
		scope := "all projects"
		if !e.Global {
			scope = e.Dir
		}
		which = append(which, e.Agent+" ("+scope+")")
		oldest = min(oldest, e.Registration)
	}
	for v := oldest + 1; v <= registrationVersion; v++ {
		if n := registrationNotes[v]; n != "" {
			news = append(news, n)
		}
	}
	text := "This gascoigne is newer than what set up " + strings.Join(which, ", ") + "."
	if len(news) > 0 {
		text += " What changed: " + strings.Join(news, "; ") + "."
	}
	current := ""
	if p.UpdateDeclined >= registrationVersion {
		current = "no"
	}
	return decision{ID: "update", Group: "updates", Title: "Update gascoigne's hooks now", Text: text, Default: "yes", Current: current,
		Options: []option{
			{"yes", "Update them now", "Rewrites the user-level ones (other keys stay, a backup is kept). Project ones need gascoigne upgrade run in the project."},
			{"no", "Not now", "Asked again with the next version."},
		}}, true
}

// setupView is what the page shows: the machine as gascoigne found it, then
// the questions.
type setupView struct {
	Version   string       `json:"version"`
	Agents    []agentState `json:"agents"`
	Questions []decision   `json:"questions"`
}

// agentState is one agent's line in the overview.
type agentState struct {
	Name  string `json:"name"`
	Scope string `json:"scope,omitempty"` // all projects, or a project dir
	State string `json:"state"`           // connected, older, not connected or declined
}

// buildSetupView lists the recorded installs (connected, or older than this
// version) and the agents on PATH that have none.
func buildSetupView(p prefs) setupView {
	v := setupView{Version: version.Number, Agents: []agentState{}, Questions: decisions(p)}
	installs := readInstalls()
	for _, e := range installs {
		state := "connected"
		if e.Registration < registrationVersion {
			state = "older"
		}
		v.Agents = append(v.Agents, agentState{Name: e.Agent, Scope: installScope(e), State: state})
	}
	for _, name := range foundAgents() {
		if slices.ContainsFunc(installs, func(e install) bool { return e.Agent == name }) {
			continue
		}
		state := "not connected"
		if slices.Contains(p.Declined, name) {
			state = "declined"
		}
		v.Agents = append(v.Agents, agentState{Name: name, State: state})
	}
	return v
}

// experimentPrefix starts the id of a question about an experimental feature.
const experimentPrefix = "experimental:"

// experiments are the opt-in features setup can turn on. Each is named as
// experimentalOn asks for it; the experimental commands in help.go are listed
// here too (a test keeps both in step).
var experiments = []struct{ name, title, text, example, how, impact string }{
	{"continuity", "Resume snapshot after compaction",
		"Keeps claude oriented after it compacts its conversation or resumes a session.",
		"gascoigne: your state before the conversation was compacted or resumed, from the transcript:\n" +
			"- last request: add a settings page\n" +
			"- open tasks:\n  - [in_progress] write tests\n" +
			"- files edited, newest first: /p/a.go, /p/b.go\n" +
			"- recent failed commands: `go test ./...`",
		"A session-start hook reads the session's transcript when claude compacts or resumes, and adds a summary to the session context. It uses only the agent's own calls (never tool output) and cuts secrets from command lines.",
		"Adds up to about 500 tokens at each compaction or resume, so the agent keeps its place instead of rediscovering it. Not yet measured against the rework it saves."},
	{"learn", "Learn from your sessions",
		"Adds the learn command: it proposes instruction lines from habits that cost context.",
		"$ gascoigne learn\nhabit  command   sessions  runs\nrerun  go test   3         7\n\n" +
			"lines for CLAUDE.local.md (-apply writes them):\n" +
			"- `go test` output arrives shortened with a hint: read the rest with\n  `gascoigne raw <id> -grep PATTERN` instead of running it again.",
		"Reads your recent runs in the ledger and finds commands run again for output that was already shortened, and large output nothing could shorten, seen in at least two sessions. -apply writes the lines to CLAUDE.local.md as a marked block; -remove takes it out.",
		"Nothing changes in your sessions until you apply lines. Then agents see a few extra instruction lines (about 30 tokens each) meant to prevent repeated reads. Its value on real ledgers is not measured yet."},
	{"prompts", "Instruction text report",
		"Adds the prompts command: what your agents are told, and what it costs.",
		"$ gascoigne prompts\nclaude sends 7.5k tokens of instructions with every request\n" +
			"source             tokens  via\nCLAUDE.md             2.7k  project\n" +
			".harness/rules.md      978  @import from CLAUDE.md\n\n" +
			"findings\n  large-import  docs/notes.md  2.5k  sent with every request",
		"Finds the instruction files an agent loads (CLAUDE.md and its imports, memory, skill and agent descriptions, AGENTS.md), counts and prices them, and flags large imports and duplicates. With -terse it can drop articles and fillers from instruction files in place (a .bak is kept).",
		"Listing changes nothing. The point is to show instruction text that you pay for on every request; -terse edits your files, so review the result."},
	{"prose", "Prose compression with a local model",
		"Downloads a small language model (274 MB) that drops low-value words from long prose, and turns the prose optimizer on.",
		"$ gascoigne prose install\ngascoigne prose: onnxruntime-linux-x64-1.29.0.tgz\n  sha256 c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba\n  downloaded, checksum matches\n\n" +
			"setup runs this download when you turn it on; the background server keeps the model loaded\n" +
			"a README paragraph: 'It sits between an agent and what the agent reads, shrinks it' -> 'sits between agent and what agent reads, shrinks it'",
		"prose install downloads the model (274 MB) and the ONNX Runtime libraries, checking their hashes. With this on, plain paragraphs of text and markdown go through the model on your GPU (CUDA or CoreML) or CPU; numbers, paths, flags, identifiers, negations, code and links are kept. The original stays retrievable with gascoigne raw.",
		"Lossy: about 8-10% fewer tokens on prose-heavy text, none on code, tables or command output. Needs the downloads and a few hundred MB of memory while it runs (in the background server, which keeps the model loaded)."},
}

// experimentDecisions are one on/off question per experimental feature. They
// always have an answer (off until turned on), so they never nag.
func experimentDecisions(p prefs) []decision {
	var ds []decision
	for _, x := range experiments {
		current := "off"
		if slices.Contains(p.Experimental, x.name) {
			current = "on"
		}
		ds = append(ds, decision{ID: experimentPrefix + x.name, Group: "experimental", Current: current, Default: "off", Optional: true,
			Title: x.title, Text: x.text, Example: x.example, How: x.how, Impact: x.impact,
			Options: []option{{"on", "Turn on", ""}, {"off", "Leave off", "The feature stays unavailable."}}})
	}
	return ds
}

// integratePrefix starts the id of a question about an agent found on PATH.
const integratePrefix = "integrate:"

// newAgents are agents on PATH that have no recorded install and were not
// declined; the setup page asks about each. gascoigne only signals them.
func newAgents(p prefs) []string {
	var out []string
	installs := readInstalls()
	for _, name := range foundAgents() {
		if !slices.ContainsFunc(installs, func(e install) bool { return e.Agent == name }) {
			out = append(out, name)
		}
	}
	return out
}

// declinedAs is the saved answer for an agent question: "no" once declined.
func declinedAs(p prefs, name string) string {
	if slices.Contains(p.Declined, name) {
		return "no"
	}
	return ""
}

func pendingDecisions(p prefs) []decision {
	return slices.DeleteFunc(decisions(p), func(d decision) bool { return d.Current != "" })
}

// applyAnswers saves the answers (decision id → option value) and writes the
// user-scope installs again so the new choices reach the configs. Unknown ids
// or values refuse the whole request.
func applyAnswers(answers map[string]string) ([]string, error) {
	p := readPrefs()
	for _, d := range decisions(p) {
		v, ok := answers[d.ID]
		if !ok {
			continue
		}
		if !slices.ContainsFunc(d.Options, func(o option) bool { return o.Value == v }) {
			return nil, fmt.Errorf("%s: %q is not an option", d.ID, v)
		}
	}
	for id := range answers {
		if !slices.ContainsFunc(decisions(p), func(d decision) bool { return d.ID == id }) {
			return nil, fmt.Errorf("unknown question %q", id)
		}
	}
	var out []string
	for id, v := range answers {
		name, ok := strings.CutPrefix(id, integratePrefix)
		if !ok {
			continue
		}
		p.Declined = slices.DeleteFunc(p.Declined, func(x string) bool { return x == name })
		if v == "no" {
			p.Declined = append(p.Declined, name)
			continue
		}
		out = append(out, integrate(name))
	}
	slices.Sort(p.Declined)
	for id, v := range answers {
		if name, ok := strings.CutPrefix(id, experimentPrefix); ok {
			p.Experimental = slices.DeleteFunc(p.Experimental, func(x string) bool { return x == name })
			if v == "on" {
				p.Experimental = append(p.Experimental, name)
			}
		}
	}
	slices.Sort(p.Experimental)
	if v, ok := answers["release"]; ok {
		p.ReleaseCheck = v
	}
	if v, ok := answers["mcp_review"]; ok {
		p.MCPReview = v
	}
	rewrite := len(out) > 0
	if v, ok := answers["update"]; ok {
		if v == "no" {
			p.UpdateDeclined = registrationVersion
		} else {
			rewrite = true
		}
	}
	for _, id := range []string{"allow", "autosync"} {
		if _, ok := answers[id]; ok {
			rewrite = true // a changed choice reaches the installed configs
		}
	}
	if v, ok := answers["allow"]; ok {
		p.Allow = v
	}
	if v, ok := answers["autosync"]; ok {
		p.Autosync = v
	}
	if err := p.save(); err != nil {
		return nil, err
	}
	if !rewrite {
		return out, nil
	}
	return append(out, rewriteInstalls()...), nil
}

// needsProseModel reports a prose experiment that is on, by this setup's
// answer or already in the config, while its files are missing or fail their
// checksums: setup then fetches them, so the optimizer works right away.
func needsProseModel(answers map[string]string) bool {
	on := cmp.Or(answers[experimentPrefix+"prose"], boolWord(experimentalOn("prose")))
	if on != "on" {
		return false
	}
	dir, err := prose.ModelDir()
	if err != nil {
		return false
	}
	err = prose.Verify(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne prose:", err)
	}
	return err != nil
}

func boolWord(on bool) string {
	if on {
		return "on"
	}
	return ""
}

// installProseModel downloads the prose model (274 MB) and the ONNX Runtime;
// progress and checksums go to stderr.
func installProseModel(ctx context.Context) error {
	dir, err := prose.ModelDir()
	if err != nil {
		return err
	}
	return prose.Install(ctx, dir, "auto")
}

// integrate sets gascoigne up for an agent the user said yes to: the same
// user-level install as gascoigne install -g <agent>.
func integrate(name string) string {
	a, ok := agents[name]
	exe, err := os.Executable()
	if !ok || err != nil {
		return name + ": could not be set up, run gascoigne install -g " + name
	}
	cmd := installedName(exe)
	if _, err := a.add(cmd, true); err != nil {
		return fmt.Sprintf("%s: could not be set up (%v), run gascoigne install -g %s", name, err, name)
	}
	if err := recordInstall(name, true, cmd, false); err != nil {
		return name + ": set up, but not recorded: " + err.Error()
	}
	return name + ": set up"
}

// rewriteInstalls writes every user-scope install again, and says which
// project installs need gascoigne install run in their project.
func rewriteInstalls() []string {
	unlock, ok := lockInstalls()
	if !ok {
		return []string{"another gascoigne is updating the installs; run gascoigne upgrade later"}
	}
	defer unlock()
	var out []string
	list := readInstalls()
	for i, e := range list {
		a, known := agents[e.Agent]
		switch {
		case !known:
		case !e.Global:
			out = append(out, fmt.Sprintf("%s in %s: run gascoigne install there to apply", e.Agent, e.Dir))
		case a.resync(e) != nil:
			out = append(out, e.Agent+": could not be updated, run gascoigne install -g")
		default:
			list[i].Registration = registrationVersion
			out = append(out, e.Agent+": updated")
		}
	}
	if err := writeInstalls(list); err != nil {
		out = append(out, "could not record the update: "+err.Error())
	}
	return out
}

// The page that changes agent settings is opened with a one-time token that
// install (or a hook) puts on disk, so only someone who can read the user's
// state dir, or was handed the link, can answer.
type setupToken struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

func setupTokenFile() string { return filepath.Join(stateDir(), "setup.json") }

func newSetupToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := setupToken{Token: hex.EncodeToString(b), Expires: time.Now().Add(24 * time.Hour)}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return "", err
	}
	data, _ := json.Marshal(t)
	return t.Token, os.WriteFile(setupTokenFile(), data, 0o600)
}

// validSetupToken reports whether got is the saved, unexpired token.
func validSetupToken(got string) bool {
	var t setupToken
	data, err := os.ReadFile(setupTokenFile())
	if err != nil || json.Unmarshal(data, &t) != nil || time.Now().After(t.Expires) {
		return false
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(t.Token)) == 1
}

// setupLink starts the server if needed and returns the page's URL with a
// fresh token.
func setupLink() (string, error) {
	if serverOff() {
		return "", errors.New("the background server is off (GASCOIGNE_SERVER=off)")
	}
	if err := serverUp(); err != nil {
		return "", err
	}
	tok, err := newSetupToken()
	if err != nil {
		return "", err
	}
	return "http://" + serverAddr() + "/setup?token=" + tok, nil
}

// askInBrowser opens the setup page when decisions are pending. A browser is
// not opened from CI or without a display; the link is returned either way.
func askInBrowser() (link string, err error) {
	link, err = setupLink()
	if err != nil {
		return "", err
	}
	if os.Getenv("CI") == "" {
		openBrowser(link)
	}
	return link, nil
}

// offerSetup is what install does at its end: ask in the browser about the
// questions still open, and say where else to answer.
func offerSetup(w io.Writer) {
	p := readPrefs()
	if len(pendingDecisions(p)) == 0 {
		return
	}
	p.AskedAt = time.Now()
	if err := p.save(); err != nil {
		fmt.Fprintln(w, "gascoigne:", err)
	}
	link, err := askInBrowser()
	if err != nil {
		fmt.Fprintln(w, "settings await your decision: run gascoigne setup")
		return
	}
	fmt.Fprintf(w, "settings await your decision: %s (or run gascoigne setup)\n", link)
}

// askInterval is how soon a session start may open the page again: a page
// closed without an answer comes back with the next session, but restarts in
// quick succession do not open a window each time.
const askInterval = 10 * time.Minute

// setupNudge is what a SessionStart hook does while installs exist and
// questions are open: open the page, again at every new session until the
// questions are answered, and tell the agent how to answer if it cannot.
// source is the SessionStart source: a compaction or /clear is not a new
// session for the user.
func setupNudge(source string) string {
	p := readPrefs()
	pending := pendingDecisions(p)
	if len(pending) == 0 || len(readInstalls()) == 0 || source == "compact" || source == "clear" ||
		time.Since(p.AskedAt) < askInterval {
		return ""
	}
	p.AskedAt = time.Now()
	if p.save() != nil {
		return ""
	}
	if _, err := askInBrowser(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
	}
	var agentsNew []string
	for _, d := range pending {
		if name, ok := strings.CutPrefix(d.ID, integratePrefix); ok {
			agentsNew = append(agentsNew, name)
		}
	}
	switch {
	case len(agentsNew) > 0:
		return "gascoigne found " + strings.Join(agentsNew, ", ") + " on PATH without a gascoigne install; it asks the user (browser, or gascoigne setup) and installs nothing by itself."
	case slices.ContainsFunc(pending, func(d decision) bool { return d.ID == "update" }):
		return "gascoigne was upgraded and its hooks installed for some agents are older than this version; it asks the user (browser, or gascoigne setup) and changes nothing by itself."
	}
	return "gascoigne has settings that await the user's decision (opened in a browser); the terminal form is: gascoigne setup."
}

// setupHandlers serves the page and its two calls, all behind the token.
func setupHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(setupHTML)
	})
	mux.HandleFunc("GET /api/setup", func(w http.ResponseWriter, r *http.Request) {
		if !validSetupToken(r.URL.Query().Get("token")) {
			http.Error(w, "this link expired or was used: run gascoigne setup", http.StatusForbidden)
			return
		}
		respond(w, buildSetupView(readPrefs()), nil)
	})
	mux.HandleFunc("POST /api/setup", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token   string            `json:"token"`
			Answers map[string]string `json:"answers"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !validSetupToken(req.Token) {
			http.Error(w, "this link expired or was used: run gascoigne setup", http.StatusForbidden)
			return
		}
		lines, err := applyAnswers(req.Answers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		os.Remove(setupTokenFile()) // one answer per link
		if needsProseModel(req.Answers) {
			// Minutes of download: not within this request.
			go func() {
				if err := installProseModel(context.Background()); err != nil {
					fmt.Fprintln(os.Stderr, "gascoigne: prose download failed, run gascoigne prose install:", err)
				}
			}()
			lines = append(lines, "prose: downloading the model (274 MB) in the background; gascoigne prose status says when it is ready")
		}
		respond(w, map[string][]string{"done": lines}, nil)
	})
}

// cmdSetup asks the open questions: in the browser when one can open here,
// else in the terminal (-t forces the terminal). -y takes the defaults; -all
// asks the answered questions again.
func cmdSetup(ctx context.Context, args []string) error {
	fset := newFlags("setup")
	yes := fset.Bool("y", false, "take the default answer to every question")
	all := fset.Bool("all", false, "also ask the questions already answered")
	term := fset.Bool("t", false, "ask in the terminal even where a browser can open")
	experimental := fset.Bool("experimental", false, "ask only about the experimental features")
	if err := fset.Parse(args); err != nil {
		return err
	}
	p := readPrefs()
	ds := pendingDecisions(p)
	if *all {
		ds = decisions(p)
	}
	if *experimental {
		ds = experimentDecisions(p)
	}
	if len(ds) == 0 {
		fmt.Println("nothing to decide; setup -all asks again")
		return repairProse(ctx, nil)
	}
	if useBrowser(*yes, *term) {
		link, err := setupLink()
		if err == nil {
			openBrowser(link)
			fmt.Printf("answer in the browser: %s\n(gascoigne setup -t asks here instead)\n", link)
			return nil
		}
		fmt.Fprintln(os.Stderr, "gascoigne:", err, "- asking here instead")
	}
	if !*yes && !isTerminal(os.Stdin) {
		return errors.New("setup: needs a browser or a terminal to ask; -y takes the defaults")
	}
	answers, err := ask(os.Stdin, os.Stdout, ds, *yes)
	if err != nil {
		return err
	}
	lines, err := applyAnswers(answers)
	for _, l := range lines {
		fmt.Println(l)
	}
	if err != nil {
		return err
	}
	return repairProse(ctx, answers)
}

// repairProse fetches the prose files again when they are missing or corrupt.
func repairProse(ctx context.Context, answers map[string]string) error {
	if !needsProseModel(answers) {
		return nil
	}
	fmt.Println("prose: downloading the model (274 MB) and ONNX Runtime")
	if err := installProseModel(ctx); err != nil {
		return fmt.Errorf("prose is on but the download failed (%w); run gascoigne prose install to retry", err)
	}
	return nil
}

// useBrowser decides where setup asks: the page, unless defaults were asked
// for, the terminal was chosen, or no browser can open here (ssh, CI, server off).
func useBrowser(yes, term bool) bool {
	return !yes && !term && hasDisplay(runtime.GOOS) && os.Getenv("CI") == "" && !serverOff()
}

// ask puts each question to w and reads the number of the chosen option from
// r; an empty line takes the default.
func ask(r io.Reader, w io.Writer, ds []decision, defaults bool) (map[string]string, error) {
	in := bufio.NewScanner(r)
	answers := map[string]string{}
	for _, d := range ds {
		answers[d.ID] = d.Default
		if defaults {
			continue
		}
		fmt.Fprintf(w, "\n%s\n%s\n", d.Title, d.Text)
		for _, part := range []struct{ name, text string }{{"Example", d.Example}, {"How it works", d.How}, {"Impact", d.Impact}} {
			if part.text != "" {
				fmt.Fprintf(w, "  %s:\n    %s\n", part.name, strings.ReplaceAll(part.text, "\n", "\n    "))
			}
		}
		for i, o := range d.Options {
			mark := " "
			if o.Value == d.Default {
				mark = "*"
			}
			fmt.Fprintf(w, " %s %d) %s\n", mark, i+1, o.Label)
			if o.Detail != "" {
				fmt.Fprintf(w, "       %s\n", o.Detail)
			}
		}
		for {
			fmt.Fprint(w, "choose [default *]: ")
			if !in.Scan() {
				return nil, errors.New("setup: no answer")
			}
			line := strings.TrimSpace(in.Text())
			if line == "" {
				break
			}
			var n int
			if _, err := fmt.Sscanf(line, "%d", &n); err == nil && n >= 1 && n <= len(d.Options) {
				answers[d.ID] = d.Options[n-1].Value
				break
			}
			fmt.Fprintf(w, "enter 1-%d\n", len(d.Options))
		}
	}
	return answers, nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// experiment is one switched-on experimental feature as the dashboard lists it.
type experiment struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// enabledExperiments lists the experimental features that are on. The prose
// model says whether it can run; the others have no state beyond being on.
func enabledExperiments() []experiment {
	out := []experiment{}
	for _, e := range experiments {
		if !experimentalOn(e.name) {
			continue
		}
		x := experiment{Name: e.name, Title: e.title, State: "on", Detail: e.text}
		if e.name == "prose" {
			st := loadProseStatus()
			x.State, x.Detail = st.State, st.Detail
		}
		out = append(out, x)
	}
	return out
}
