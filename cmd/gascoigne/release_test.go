package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

// site serves latest.json with body and counts the requests it gets.
func site(t *testing.T, body string, status int) (url string, hits *atomic.Int32) {
	t.Helper()
	hits = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/web/latest.json" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "gascoigne" || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("the request must carry nothing about the machine: %v ?%s", r.Header, r.URL.RawQuery)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/web/", hits
}

func TestFetchRelease(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		wantErr    string
		wantURL    string
	}{
		{"id only: the default page", `{"id":"0.3.0","released":"2026-10-12"}`, 200, "", "/web/releases/0.3.0/"},
		{"a page on the site", `{"id":"0.3.0","page":"whats-new/"}`, 200, "", "/web/whats-new/"},
		{"another host", `{"id":"0.3.0","page":"https://evil.example/x"}`, 200, "not on the site", ""},
		{"above the site's path", `{"id":"0.3.0","page":"/other/"}`, 200, "not on the site", ""},
		{"not a release id", `{"id":"latest"}`, 200, "not a release id", ""},
		{"a dev id", `{"id":"0.3.0-dev"}`, 200, "not a release id", ""},
		{"not json", `<html>`, 200, "latest.json", ""},
		{"missing", ``, 404, "404", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := site(t, tt.body, tt.status)
			r, err := fetchRelease(t.Context(), base)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !strings.HasSuffix(r.URL, tt.wantURL) || r.ID != "0.3.0" {
				t.Fatalf("%+v, %v", r, err)
			}
		})
	}
	if _, err := fetchRelease(t.Context(), "ftp://x/"); err == nil {
		t.Error("only web addresses")
	}
}

func TestFetchReleaseReadsLittle(t *testing.T) {
	base, _ := site(t, `{"id":"0.3.0","page":"`+strings.Repeat("a", 10000)+`"}`, 200)
	if _, err := fetchRelease(t.Context(), base); err == nil {
		t.Error("a file over 4 KB is cut and refused")
	}
}

func TestCheckReleaseOncePerDay(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_RELEASE_CHECK", "")
	base, hits := site(t, `{"id":"0.3.0"}`, 200)
	t.Setenv("GASCOIGNE_RELEASE_URL", base)

	if err := checkRelease(t.Context()); err != nil || hits.Load() != 1 {
		t.Fatalf("first check: %v, hits %d", err, hits.Load())
	}
	if r := readRelease(); r.ID != "0.3.0" || r.CheckedAt.IsZero() {
		t.Fatalf("saved: %+v", r)
	}
	if err := checkRelease(t.Context()); err != nil || hits.Load() != 1 {
		t.Fatalf("within an hour nothing is read: hits %d", hits.Load())
	}
	r := readRelease()
	r.CheckedAt = time.Now().Add(-releaseEvery - time.Minute)
	if err := r.save(); err != nil {
		t.Fatal(err)
	}
	if err := checkRelease(t.Context()); err != nil || hits.Load() != 2 {
		t.Fatalf("after an hour it is read again: hits %d", hits.Load())
	}
}

func TestCheckReleaseFailureKeepsTheLastAnswerAndRetriesSoon(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_RELEASE_CHECK", "")
	good := release{ID: "0.3.0", URL: "https://x/", CheckedAt: time.Now().Add(-releaseEvery - time.Minute)}
	if err := good.save(); err != nil {
		t.Fatal(err)
	}
	base, hits := site(t, ``, 500)
	t.Setenv("GASCOIGNE_RELEASE_URL", base)
	if err := checkRelease(t.Context()); err == nil {
		t.Fatal("the failure is reported")
	}
	got := readRelease()
	if got.ID != "0.3.0" {
		t.Errorf("the last good answer stays: %+v", got)
	}
	if wait := releaseEvery - time.Since(got.CheckedAt); wait > releaseRetry+time.Minute || wait < releaseRetry-time.Minute {
		t.Errorf("tried again in about an hour, not %v", wait)
	}
	if checkRelease(t.Context()); hits.Load() != 1 {
		t.Errorf("not hammered: %d requests", hits.Load())
	}
}

func TestReleaseCheckOffMakesNoRequest(t *testing.T) {
	sandbox(t)
	base, hits := site(t, `{"id":"0.3.0"}`, 200)
	t.Setenv("GASCOIGNE_RELEASE_URL", base)
	for name, set := range map[string]func(){
		"env": func() { t.Setenv("GASCOIGNE_RELEASE_CHECK", "off") },
		"setup": func() {
			t.Setenv("GASCOIGNE_RELEASE_CHECK", "")
			_, _ = applyAnswers(map[string]string{"release": "off"})
		},
	} {
		set()
		if err := checkRelease(t.Context()); err != nil || hits.Load() != 0 || releaseCheckOn() {
			t.Fatalf("%s: off means no request (hits %d)", name, hits.Load())
		}
	}
}

func TestReleaseNudgeOpensThePageOnce(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_RELEASE_CHECK", "")
	t.Setenv("CI", "true") // never open a real browser from a test
	save := func(id string) {
		t.Helper()
		if err := (release{ID: id, URL: "https://site/releases/" + id + "/", CheckedAt: time.Now()}).save(); err != nil {
			t.Fatal(err)
		}
	}
	if releaseNudge("startup") != "" {
		t.Fatal("nothing known yet")
	}
	save(version.Number) // the release being run
	if releaseNudge("startup") != "" {
		t.Fatal("not newer than this one")
	}
	save("99.0.0")
	if releaseNudge("compact") != "" || releaseNudge("clear") != "" {
		t.Fatal("a compaction or /clear is not a new session")
	}
	note := releaseNudge("startup")
	if !strings.Contains(note, "99.0.0 is out") || !strings.Contains(note, "https://site/releases/99.0.0/") || !strings.Contains(note, "gascoigne upgrade") {
		t.Fatalf("note: %q", note)
	}
	if readPrefs().SeenRelease != "99.0.0" {
		t.Error("the shown release is remembered")
	}
	if releaseNudge("resume") != "" {
		t.Fatal("a release page opens once")
	}
	save("99.1.0")
	if !strings.Contains(releaseNudge("startup"), "99.1.0") {
		t.Fatal("the next release opens its own page")
	}
	t.Setenv("GASCOIGNE_RELEASE_CHECK", "off")
	save("99.2.0")
	if releaseNudge("startup") != "" {
		t.Fatal("off: no page")
	}
}

func TestReleaseQuestionIsOptionalAndOnByDefault(t *testing.T) {
	sandbox(t)
	d := releaseDecision(prefs{})
	if !d.Optional || d.Current != "on" || d.Group != "updates" {
		t.Fatalf("%+v", d)
	}
	for _, p := range pendingDecisions(prefs{}) {
		if p.ID == "release" {
			t.Error("the release check never counts as an open question")
		}
	}
	if !strings.Contains(d.Options[0].Detail, "latest.json") || !strings.Contains(d.Options[0].Detail, "IP address") {
		t.Errorf("say what is read and what the host sees: %q", d.Options[0].Detail)
	}
	if got := releaseDecision(prefs{ReleaseCheck: "off"}); got.Current != "off" {
		t.Errorf("saved off: %+v", got)
	}
}

// A page that could not open (no desktop session) is tried again at the next
// session start; one that opened is not.
func TestReleaseNudgeRetriesWhenThePageDoesNotOpen(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_RELEASE_CHECK", "")
	t.Setenv("CI", "")
	var opened []string
	works := false
	prev := openPage
	openPage = func(u string) bool { opened = append(opened, u); return works }
	t.Cleanup(func() { openPage = prev })
	if err := (release{ID: "99.0.0", URL: "https://site/releases/99.0.0/", CheckedAt: time.Now()}).save(); err != nil {
		t.Fatal(err)
	}
	if note := releaseNudge("startup"); !strings.Contains(note, "99.0.0 is out") {
		t.Fatalf("the agent is told even when no page opens: %q", note)
	}
	if readPrefs().SeenRelease == "99.0.0" {
		t.Fatal("an unopened page is not remembered as seen")
	}
	works = true
	if releaseNudge("startup") == "" || len(opened) != 2 {
		t.Fatalf("tried again: opened %v", opened)
	}
	if readPrefs().SeenRelease != "99.0.0" {
		t.Fatal("remembered once opened")
	}
	if releaseNudge("startup") != "" || len(opened) != 2 {
		t.Fatalf("opens once: %v", opened)
	}
}
