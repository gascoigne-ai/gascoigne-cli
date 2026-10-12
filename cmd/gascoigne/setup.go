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
	"slices"
	"strings"
	"time"
)

//go:embed setup.html
var setupHTML []byte

// A decision is one question gascoigne asks the user before it changes
// something beyond its own files: in the browser (/setup) or in the terminal
// (gascoigne setup). Nothing it covers is done until the user answers.
type decision struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Options []option `json:"options"`
	Default string   `json:"default"`
	Current string   `json:"current"` // the saved answer, "" = not decided
}

type option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// decisions are the questions, with the saved answers filled in.
func decisions(p prefs) []decision {
	ds := []decision{
		{ID: "allow", Current: p.Allow, Default: "local",
			Title: "Fewer approval prompts",
			Text: "Claude asks before each use of gascoigne's tools. Adds allow rules to claude's settings " +
				"(unwrap removes exactly those). Only fetch reaches the network.",
			Options: []option{
				{"local", "Allow retrieve, show, stats and compress (read-only, local)"},
				{"all", "Also allow fetch (reads web pages and APIs)"},
				{"none", "Keep asking"},
			}},
		{ID: "autosync", Current: p.Autosync, Default: "on",
			Title: "Update hooks after an upgrade",
			Text: "When a new gascoigne adds or changes hooks, rewrite the ones gascoigne installed in your " +
				"user-level agent configs (other keys stay, a backup is kept). Project files are never touched " +
				"automatically.",
			Options: []option{
				{"on", "Keep my installed hooks up to date"},
				{"off", "Only change my configs when I run gascoigne install"},
			}},
	}
	for _, name := range newAgents(p) {
		ds = append(ds, decision{ID: integratePrefix + name, Default: "yes", Current: declinedAs(p, name),
			Title: "Use gascoigne with " + name,
			Text: name + " is on PATH but gascoigne is not set up for it. Yes writes gascoigne's hooks and MCP server into " +
				"your user-level " + name + " config (a backup is kept; unwrap removes them). Nothing is installed without your answer.",
			Options: []option{{"yes", "Set it up for " + name}, {"no", "Not for " + name}}})
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
	if v, ok := answers["allow"]; ok {
		p.Allow = v
	}
	if v, ok := answers["autosync"]; ok {
		p.Autosync = v
	}
	if err := p.save(); err != nil {
		return nil, err
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
		return []string{"another gascoigne is updating the installs; run gascoigne install -sync later"}
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
		respond(w, decisions(readPrefs()), nil)
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

// cmdSetup asks the open questions in the terminal. -y takes the defaults;
// -all asks the answered ones again too.
func cmdSetup(_ context.Context, args []string) error {
	fset := newFlags("setup")
	yes := fset.Bool("y", false, "take the default answer to every question")
	all := fset.Bool("all", false, "also ask the questions already answered")
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
	if !*yes && !isTerminal(os.Stdin) {
		return errors.New("setup: needs a terminal to ask; -y takes the defaults")
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
