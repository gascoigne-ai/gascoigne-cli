package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

const sheetA = `{"version":1,"models":{"acme-large":{"in":3,"out":15,"cache_read":0.3,"cache_write":3.75,"tokenizer":"claude"}}}`

func price(v float64) *float64 { return &v }

func lookup(t *testing.T, result Result, id string) (cost.Model, string) {
	t.Helper()
	cost.SetOverrides(result.Overrides)
	t.Cleanup(func() { cost.SetOverrides(nil) })
	return cost.LookupSource(id)
}

func clock(now *time.Time) func() time.Time { return func() time.Time { return *now } }

func TestPrecedenceStaticJSONAPIScript(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sheet.json")
	os.WriteFile(file, []byte(`{"models":{"m-all":{"in":2,"out":2},"m-json":{"in":2,"out":2}}}`), 0o600)
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Write([]byte(`{"models":{"m-all":{"in":3,"out":3},"m-api":{"in":3,"out":3}}}`))
	}))
	defer api.Close()
	now := time.Now()
	cfg := Config{
		Static: map[string]cost.PriceItem{"m-all": {In: price(1), Out: price(1)}},
		File:   file, URL: api.URL, StateDir: dir, Now: clock(&now),
	}
	if err := Refresh(t.Context(), cfg, false); err != nil {
		t.Fatal(err)
	}
	result := Load(cfg)
	for id, want := range map[string]struct {
		in     float64
		source string
	}{
		"m-all":  {1, "static"},
		"m-json": {2, "json"},
		"m-api":  {3, "api"},
	} {
		m, source := lookup(t, result, id)
		if m.Input != want.in || source != want.source {
			t.Errorf("%s: in %v from %q, want %v from %q", id, m.Input, source, want.in, want.source)
		}
	}
	if m, source := lookup(t, Load(Config{StateDir: dir}), "claude-haiku-5-5"); source != cost.SourceBuiltIn || m.Input != 0.10 {
		t.Errorf("built-in lost: %+v %q", m, source)
	}
}

func TestAPIIsCachedForTheTTLAndSurvivesFailure(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if fail.Load() {
			http.Error(rw, "down", http.StatusServiceUnavailable)
			return
		}
		rw.Write([]byte(sheetA))
	}))
	defer api.Close()
	now := time.Now()
	cfg := Config{URL: api.URL, StateDir: t.TempDir(), TTL: 24 * time.Hour, Now: clock(&now)}

	if err := Refresh(t.Context(), cfg, false); err != nil || calls.Load() != 1 {
		t.Fatalf("first refresh: %v, %d calls", err, calls.Load())
	}
	now = now.Add(23 * time.Hour)
	if load := Load(cfg); load.Stale {
		t.Fatal("fresh within the TTL")
	}
	Refresh(t.Context(), cfg, false)
	if calls.Load() != 1 {
		t.Fatalf("refreshed inside the TTL: %d calls", calls.Load())
	}
	now = now.Add(2 * time.Hour)
	fail.Store(true)
	if load := Load(cfg); !load.Stale {
		t.Fatal("stale after the TTL")
	}
	if err := Refresh(t.Context(), cfg, false); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("failure not reported: %v", err)
	}
	load := Load(cfg)
	if m, source := lookup(t, load, "acme-large"); m.Input != 3 || source != "api" {
		t.Fatalf("last good prices lost after a failed refresh: %+v %q", m, source)
	}
	if len(load.Layers) != 1 || !strings.Contains(load.Layers[0].Error, "503") {
		t.Fatalf("status = %+v", load.Layers)
	}
	// A failure backs off for an hour.
	calls.Store(0)
	now = now.Add(30 * time.Minute)
	if load := Load(cfg); load.Stale {
		t.Fatal("retry sooner than the backoff")
	}
	Refresh(t.Context(), cfg, false)
	if calls.Load() != 0 {
		t.Fatalf("retried inside the backoff: %d calls", calls.Load())
	}
	now = now.Add(31 * time.Minute)
	fail.Store(false)
	if err := Refresh(t.Context(), cfg, false); err != nil || calls.Load() != 1 {
		t.Fatalf("recovery: %v, %d calls", err, calls.Load())
	}
	if Load(cfg).Layers[0].Error != "" {
		t.Fatal("error stays after a good run")
	}
}

func TestAPIRevalidatesWith304AndSendsHeader(t *testing.T) {
	t.Setenv("PRICES_AUTH", "Authorization: Bearer secret")
	var sawAuth, sawETag string
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
		sawAuth, sawETag = request.Header.Get("Authorization"), request.Header.Get("If-None-Match")
		if sawETag == `"v1"` {
			rw.WriteHeader(http.StatusNotModified)
			return
		}
		rw.Header().Set("ETag", `"v1"`)
		rw.Write([]byte(sheetA))
	}))
	defer api.Close()
	now := time.Now()
	cfg := Config{URL: api.URL, HeaderEnv: "PRICES_AUTH", StateDir: t.TempDir(), Now: clock(&now)}
	if err := Refresh(t.Context(), cfg, false); err != nil || sawAuth != "Bearer secret" {
		t.Fatalf("first: %v, auth %q", err, sawAuth)
	}
	now = now.Add(25 * time.Hour)
	if err := Refresh(t.Context(), cfg, false); err != nil || sawETag != `"v1"` {
		t.Fatalf("second: %v, etag %q", err, sawETag)
	}
	load := Load(cfg)
	if load.Stale || !load.Layers[0].Fetched.Equal(now) {
		t.Fatalf("a 304 must renew the cache: %+v", load.Layers[0])
	}
	if m, _ := lookup(t, load, "acme-large"); m.Input != 3 {
		t.Fatalf("prices lost on 304: %+v", m)
	}
}

func TestAPIRejectsBadBodyAndKeepsOldPrices(t *testing.T) {
	body := sheetA
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.Write([]byte(body)) }))
	defer api.Close()
	now := time.Now()
	cfg := Config{URL: api.URL, StateDir: t.TempDir(), Now: clock(&now)}
	if err := Refresh(t.Context(), cfg, false); err != nil {
		t.Fatal(err)
	}
	body = `<html>maintenance</html>`
	if err := Refresh(t.Context(), cfg, true); err == nil || !strings.Contains(err.Error(), "api prices") {
		t.Fatalf("bad body accepted: %v", err)
	}
	if m, source := lookup(t, Load(cfg), "acme-large"); m.Input != 3 || source != "api" {
		t.Fatalf("old prices dropped: %+v", m)
	}
}

func TestManualTTLNeverRunsByItselfButForceDoes(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { calls.Add(1); rw.Write([]byte(sheetA)) }))
	defer api.Close()
	now := time.Now()
	cfg := Config{URL: api.URL, StateDir: t.TempDir(), Manual: true, Now: clock(&now)}
	if Load(cfg).Stale {
		t.Fatal("manual: never stale")
	}
	Refresh(t.Context(), cfg, false)
	if calls.Load() != 0 {
		t.Fatal("manual refreshed by itself")
	}
	if err := Refresh(t.Context(), cfg, true); err != nil || calls.Load() != 1 {
		t.Fatalf("run now: %v, %d", err, calls.Load())
	}
}

// writeScript makes an executable shell script printing what its body says.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "translate.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScriptTranslatesForeignFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "foreign.csv")
	os.WriteFile(file, []byte("big-model,3,15\n"), 0o600)
	// Foreign CSV on stdin -> gascoigne JSON.
	script := writeScript(t, `IFS=, read id in out; printf '{"models":{"%s":{"in":%s,"out":%s}}}' "$id" "$in" "$out"`)
	now := time.Now()
	cfg := Config{File: file, Translate: script, StateDir: dir, Now: clock(&now)}
	if load := Load(cfg); !load.Stale {
		t.Fatal("an untranslated file is stale")
	}
	if err := Refresh(t.Context(), cfg, false); err != nil {
		t.Fatal(err)
	}
	load := Load(cfg)
	if m, source := lookup(t, load, "big-model"); m.Input != 3 || m.Output != 15 || source != "json" || load.Stale {
		t.Fatalf("translated: %+v from %q stale %v", m, source, load.Stale)
	}
	// An unchanged file does not rerun the script.
	os.WriteFile(script, []byte("#!/bin/sh\nexit 9\n"), 0o700)
	if err := Refresh(t.Context(), cfg, false); err != nil {
		t.Fatalf("script reran for an unchanged file: %v", err)
	}
	// A changed file does.
	os.WriteFile(file, []byte("other,1,2\n"), 0o600)
	later := time.Now().Add(time.Minute)
	os.Chtimes(file, later, later)
	if load := Load(cfg); !load.Stale {
		t.Fatal("a changed file is stale")
	}
}

func TestScriptAloneRunsOncePerTTL(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	script := writeScript(t, `echo x >> `+counter+`; printf '{"models":{"s-model":{"in":1,"out":2}}}'`)
	now := time.Now()
	cfg := Config{Translate: script, StateDir: dir, TTL: 24 * time.Hour, Now: clock(&now)}
	runs := func() int {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "x")
	}
	for range 3 {
		Refresh(t.Context(), cfg, false)
	}
	if runs() != 1 {
		t.Fatalf("script ran %d times inside the TTL", runs())
	}
	if m, source := lookup(t, Load(cfg), "s-model"); m.Input != 1 || source != "script" {
		t.Fatalf("script prices: %+v %q", m, source)
	}
	now = now.Add(25 * time.Hour)
	Refresh(t.Context(), cfg, false)
	if runs() != 2 {
		t.Fatalf("script ran %d times after the TTL", runs())
	}
}

func TestScriptFailuresKeepLastGoodPrices(t *testing.T) {
	dir := t.TempDir()
	good := writeScript(t, `printf '{"models":{"s-model":{"in":1,"out":2}}}'`)
	now := time.Now()
	cfg := Config{Translate: good, StateDir: dir, Timeout: 500 * time.Millisecond, Now: clock(&now)}
	if err := Refresh(t.Context(), cfg, false); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"exit code": `echo boom >&2; exit 3`,
		"timeout":   `sleep 5`,
		"oversize":  `head -c 2000000 /dev/zero`,
		"bad json":  `echo not json`,
	} {
		cfg.Translate = writeScript(t, body)
		err := Refresh(t.Context(), cfg, true)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if name == "exit code" && !strings.Contains(err.Error(), "boom") {
			t.Errorf("stderr not reported: %v", err)
		}
		load := Load(cfg)
		if m, _ := lookup(t, load, "s-model"); m.Input != 1 {
			t.Errorf("%s: last good prices lost", name)
		}
		if load.Layers[0].Error == "" {
			t.Errorf("%s: status shows no error", name)
		}
	}
}

func TestSplitArgs(t *testing.T) {
	for in, want := range map[string]string{
		`/bin/tr a b`:                 "/bin/tr|a|b",
		`"/opt/my tools/x" --k 'a b'`: "/opt/my tools/x|--k|a b",
		`python3  script.py   `:       "python3|script.py",
		`a\ b c`:                      "a b|c",
	} {
		got, err := SplitArgs(in)
		if err != nil || strings.Join(got, "|") != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := SplitArgs(`"open`); err == nil {
		t.Error("unterminated quote accepted")
	}
	if _, err := SplitArgs("  "); err == nil {
		t.Error("empty command accepted")
	}
}

func TestParseTTL(t *testing.T) {
	for in, want := range map[string]struct {
		ttl    time.Duration
		manual bool
		warn   bool
	}{
		"":       {24 * time.Hour, false, false},
		"12h":    {12 * time.Hour, false, false},
		"168h":   {168 * time.Hour, false, false},
		"30m":    {time.Hour, false, true},
		"0":      {0, true, false},
		"manual": {0, true, false},
	} {
		ttl, manual, warn, err := ParseTTL(in)
		if err != nil || ttl != want.ttl || manual != want.manual || (warn != "") != want.warn {
			t.Errorf("%q = %v %v %q %v; want %+v", in, ttl, manual, warn, err, want)
		}
	}
	if _, _, _, err := ParseTTL("soon"); err == nil {
		t.Error("garbage accepted")
	}
	if _, _, _, err := ParseTTL("-5h"); err == nil {
		t.Error("negative accepted")
	}
}

func TestStaticAndFileErrorsAreReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.json")
	os.WriteFile(file, []byte(`{"models":{"x":{"in":-1,"out":1}}}`), 0o600)
	load := Load(Config{
		Static: map[string]cost.PriceItem{"neg": {In: price(-1), Out: price(1)}},
		File:   file, StateDir: dir,
	})
	if len(load.Overrides) != 0 {
		t.Fatalf("bad prices applied: %+v", load.Overrides)
	}
	var sources []string
	for _, layer := range load.Layers {
		if layer.Error == "" {
			t.Errorf("%s: error not reported", layer.Name)
		}
		sources = append(sources, layer.Name)
	}
	if strings.Join(sources, ",") != "static,json" {
		t.Fatalf("layers = %v", sources)
	}
	_ = context.Background
}

func TestFileThatIsNotARegularSmallFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.json")
	os.WriteFile(big, []byte(`{"models":{}}`+strings.Repeat(" ", MaxBytes+1)), 0o600)
	for name, file := range map[string]string{"directory": dir, "over the limit": big, "missing": filepath.Join(dir, "none.json")} {
		load := Load(Config{File: file, StateDir: dir})
		if len(load.Layers) != 1 || load.Layers[0].Error == "" || len(load.Overrides) != 0 {
			t.Errorf("%s: %+v", name, load)
		}
	}
}

func TestURLWithCredentialsOrAnOddSchemeIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, url := range []string{"https://user:pw@prices.example.com/p.json", "file:///etc/passwd", "ftp://x/y", "https:///nohost"} {
		cfg := Config{URL: url, StateDir: dir}
		if err := Refresh(t.Context(), cfg, true); err == nil {
			t.Errorf("%s accepted", url)
		}
	}
}

func TestRedirectPolicy(t *testing.T) {
	request := func(url string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	via := func(urls ...string) []*http.Request {
		out := make([]*http.Request, len(urls))
		for i, u := range urls {
			out[i] = request(u)
		}
		return out
	}
	if err := redirectPolicy(request("https://b.example/p"), via("https://a.example/p")); err != nil {
		t.Fatalf("a plain redirect is fine: %v", err)
	}
	if err := redirectPolicy(request("http://b.example/p"), via("https://a.example/p")); err == nil {
		t.Fatal("https to http downgrade followed")
	}
	if err := redirectPolicy(request("https://u:p@b.example/p"), via("https://a.example/p")); err == nil {
		t.Fatal("redirect to a URL with credentials followed")
	}
	if err := redirectPolicy(request("ftp://b.example/p"), via("https://a.example/p")); err == nil {
		t.Fatal("redirect to another scheme followed")
	}
	if err := redirectPolicy(request("https://e.example/p"), via("https://a/p", "https://b/p", "https://c/p", "https://d/p")); err == nil {
		t.Fatal("a long redirect chain followed")
	}
}

func TestACommandNotOnPathIsRefusedBeforeItRuns(t *testing.T) {
	cfg := Config{Translate: "no-such-translator-xyz --flag", StateDir: t.TempDir()}
	err := Refresh(t.Context(), cfg, true)
	if err == nil || !strings.Contains(err.Error(), "no-such-translator-xyz") {
		t.Fatalf("err = %v", err)
	}
}
