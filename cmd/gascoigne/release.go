package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

// Release notice. The project website keeps a small file, latest.json, with
// the id of the latest release and the page that tells about it. The
// session and prompt hooks (and the dashboard command) read it every 15
// minutes; the first one after a newer release appeared opens that page once.
// The request is a plain GET: nothing identifying is sent. On by default; $GASCOIGNE_RELEASE_CHECK=off or
// the setup question turns it off.

// releaseSite is the website's address; $GASCOIGNE_RELEASE_URL replaces it
// (tests, a mirror).
const releaseSite = "https://gascoigne-ai.github.io/"

// releaseEvery is how long a result stands before the file is read again.
const releaseEvery = 15 * time.Minute

// releaseRetry is how soon a failed read is tried again.
const releaseRetry = 15 * time.Minute

var releaseID = lazyre.New(`^\d+\.\d+\.\d+$`)

func releaseBase() string {
	b := cmp.Or(os.Getenv("GASCOIGNE_RELEASE_URL"), releaseSite)
	if !strings.HasSuffix(b, "/") {
		b += "/"
	}
	return b
}

// releaseCheckOn reports whether the website is read at all.
func releaseCheckOn() bool {
	return os.Getenv("GASCOIGNE_RELEASE_CHECK") != "off" && readPrefs().ReleaseCheck != "off"
}

// A release is what the last read of latest.json said.
type release struct {
	ID        string    `json:"id"`
	Released  string    `json:"released,omitempty"`
	URL       string    `json:"url"`        // the page about it, absolute
	CheckedAt time.Time `json:"checked_at"` // when the file was last read
}

func releaseFile() string { return filepath.Join(stateDir(), "release.json") }

func readRelease() release {
	var r release
	if data, err := os.ReadFile(releaseFile()); err == nil {
		json.Unmarshal(data, &r)
	}
	return r
}

func (r release) save() error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	tmp := releaseFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, releaseFile())
}

// newer is true when this release is later than the running gascoigne.
func (r release) newer() bool { return version.Newer(r.ID, version.Number) }

// fetchRelease reads base+latest.json. The id must be a plain version, and the
// page must stay on the site: a file that points elsewhere is refused.
func fetchRelease(ctx context.Context, base string) (release, error) {
	site, err := url.Parse(base)
	if err != nil || site.Host == "" || (site.Scheme != "https" && site.Scheme != "http") {
		return release{}, fmt.Errorf("release site %q is not a web address", base)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"latest.json", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("User-Agent", "gascoigne") // no version, no id: nothing about this machine
	client := &http.Client{CheckRedirect: func(r *http.Request, _ []*http.Request) error {
		if r.URL.Host != site.Host {
			return errors.New("redirect to another host")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("latest.json: %s", resp.Status)
	}
	var f struct {
		ID       string `json:"id"`
		Released string `json:"released"`
		Page     string `json:"page"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&f); err != nil {
		return release{}, fmt.Errorf("latest.json: %w", err)
	}
	if !releaseID.MatchString(f.ID) {
		return release{}, fmt.Errorf("latest.json: %q is not a release id", f.ID)
	}
	page, err := site.Parse(cmp.Or(f.Page, "releases/"+f.ID+"/"))
	if err != nil || page.Host != site.Host || page.Scheme != site.Scheme || !strings.HasPrefix(page.Path, site.Path) {
		return release{}, fmt.Errorf("latest.json: page %q is not on the site", f.Page)
	}
	return release{ID: f.ID, Released: f.Released, URL: page.String()}, nil
}

// checkRelease reads the website when 15 minutes have passed since the last read.
// A failed read is tried again in 15 minutes; the last good answer stays.
func checkRelease(ctx context.Context) error {
	if !releaseCheckOn() {
		return nil
	}
	last := readRelease()
	if time.Since(last.CheckedAt) < releaseEvery {
		return nil
	}
	got, err := fetchRelease(ctx, releaseBase())
	if err != nil {
		last.CheckedAt = time.Now().Add(releaseRetry - releaseEvery)
		last.save()
		return err
	}
	got.CheckedAt = time.Now()
	return got.save()
}

// checkReleaseQuick is checkRelease for a command a person waits on: a slow
// site costs at most two seconds, once per releaseEvery.
func checkReleaseQuick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := checkRelease(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: release check:", err)
	}
}

// openPage opens a release page; a test replaces it.
var openPage = tryOpenBrowser

// releaseNudge is what a session start does when a newer release than the
// running one is known (from the file checkRelease saved): open its
// page once, and tell the agent. A compaction or /clear is not a new session.
func releaseNudge(source string) string {
	if source == "compact" || source == "clear" || !releaseCheckOn() {
		return ""
	}
	r := readRelease()
	p := readPrefs()
	if !r.newer() || p.SeenRelease == r.ID {
		return ""
	}
	// Remembered only once the page opened (or opening is off), so a session
	// without a desktop does not use up the release.
	if os.Getenv("CI") != "" || openPage(r.URL) {
		p.SeenRelease = r.ID
		if p.save() != nil {
			return ""
		}
	}
	return fmt.Sprintf("gascoigne %s is out (this machine runs %s): %s. After installing it, run gascoigne upgrade.", r.ID, version.Number, r.URL)
}

// releaseDecision is the setup question about the check; it is optional and
// answered "on" until the user says otherwise.
func releaseDecision(p prefs) decision {
	current := "on"
	if p.ReleaseCheck == "off" {
		current = "off"
	}
	return decision{ID: "release", Group: "updates", Optional: true, Current: current, Default: "on",
		Title: "Tell me about new releases",
		Text:  "gascoigne can notice when a new version is out and open its page once.",
		Options: []option{
			{"on", "Check every 15 minutes", "gascoigne reads " + releaseBase() + "latest.json hourly. It is a plain request: nothing about you or this machine is sent, but the site's host sees your IP address as for any web page. A newer release opens its page once the next time a session starts or you send a prompt."},
			{"off", "Never check", "No request is made. Look for releases on the website or on GitHub yourself."},
		}}
}
