package main

import (
	"bufio"
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
	return ds
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
	v := setupView{Version: version.String(), Agents: []agentState{}, Questions: decisions(p)}
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
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
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
	p.AskedRegistration = registrationVersion
	p.Offered = append(p.Offered, newAgents(p)...)
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

// setupNudge is what a SessionStart hook does when installs exist and
// questions are open: ask in the browser, once per registration version, and
// tell the agent how to answer if it cannot.
func setupNudge() string {
	p := readPrefs()
	if len(readInstalls()) == 0 {
		return ""
	}
	fresh := slices.DeleteFunc(newAgents(p), func(n string) bool {
		return slices.Contains(p.Offered, n) || slices.Contains(p.Declined, n)
	})
	own := slices.DeleteFunc(pendingDecisions(p), func(d decision) bool { return strings.HasPrefix(d.ID, integratePrefix) })
	if len(fresh) == 0 && (len(own) == 0 || p.AskedRegistration >= registrationVersion) {
		return ""
	}
	p.AskedRegistration = registrationVersion
	p.Offered = append(p.Offered, fresh...)
	if p.save() != nil {
		return ""
	}
	if _, err := askInBrowser(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
	}
	if len(fresh) > 0 {
		return "gascoigne found " + strings.Join(fresh, ", ") + " on PATH without a gascoigne install; it asks the user (browser, or gascoigne setup) and installs nothing by itself."
	}
	if len(staleInstalls()) > 0 && p.UpdateDeclined < registrationVersion && p.Autosync != "on" {
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
		respond(w, map[string][]string{"done": lines}, nil)
	})
}

// cmdSetup asks the open questions: in the browser when one can open here,
// else in the terminal (-t forces the terminal). -y takes the defaults; -all
// asks the answered questions again.
func cmdSetup(_ context.Context, args []string) error {
	fset := newFlags("setup")
	yes := fset.Bool("y", false, "take the default answer to every question")
	all := fset.Bool("all", false, "also ask the questions already answered")
	term := fset.Bool("t", false, "ask in the terminal even where a browser can open")
	if err := fset.Parse(args); err != nil {
		return err
	}
	p := readPrefs()
	ds := pendingDecisions(p)
	if *all {
		ds = decisions(p)
	}
	if len(ds) == 0 {
		fmt.Println("nothing to decide; setup -all asks again")
		return nil
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
	return err
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
