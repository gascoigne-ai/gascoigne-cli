package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

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
	return t.Token, saveJSON(setupTokenFile(), t)
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
	if err := serverUp(context.Background()); err != nil {
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
func offerSetup(out io.Writer) {
	p := readPrefs()
	if len(pendingDecisions(p)) == 0 {
		return
	}
	p.AskedAt = time.Now()
	if err := p.save(); err != nil {
		fmt.Fprintln(out, "gascoigne:", err)
	}
	link, err := askInBrowser()
	if err != nil {
		fmt.Fprintln(out, "settings await your decision: run gascoigne setup")
		return
	}
	fmt.Fprintf(out, "settings await your decision: %s (or run gascoigne setup)\n", link)
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
	upgraded := p.SetupVersion != version.Number
	if (len(pending) == 0 && !upgraded) || len(readInstalls()) == 0 || source == "compact" || source == "clear" ||
		time.Since(p.AskedAt) < askInterval {
		return ""
	}
	p.AskedAt = time.Now()
	p.SetupVersion = version.Number
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
	case len(pending) == 0:
		return "gascoigne was upgraded to " + version.Number + "; the setup page was opened in a browser to finish the update (or run gascoigne setup -all)."
	case len(agentsNew) > 0:
		return "gascoigne found " + strings.Join(agentsNew, ", ") + " on PATH without a gascoigne install; it asks the user (browser, or gascoigne setup) and installs nothing by itself."
	case slices.ContainsFunc(pending, func(choice decision) bool { return choice.ID == "update" }):
		return "gascoigne was upgraded and its hooks installed for some agents are older than this version; it asks the user (browser, or gascoigne setup) and changes nothing by itself."
	}
	return "gascoigne has settings that await the user's decision (opened in a browser); the terminal form is: gascoigne setup."
}
