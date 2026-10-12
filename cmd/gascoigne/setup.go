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
	return []decision{
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
	if v, ok := answers["allow"]; ok {
		p.Allow = v
	}
	if v, ok := answers["autosync"]; ok {
		p.Autosync = v
	}
	if err := p.save(); err != nil {
		return nil, err
	}
	return rewriteInstalls(), nil
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
	if len(pendingDecisions(p)) == 0 || p.AskedRegistration >= registrationVersion || len(readInstalls()) == 0 {
		return ""
	}
	p.AskedRegistration = registrationVersion
	if p.save() != nil {
		return ""
	}
	if _, err := askInBrowser(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
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
