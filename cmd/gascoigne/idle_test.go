package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivity(t *testing.T) {
	start := time.Now()
	a := newActivity(start)
	release := make(chan struct{})
	h := a.track(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			<-release
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/health", nil))
	if !a.last.Equal(start) {
		t.Fatal("a health check counted as use")
	}
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/stream", nil))
		close(done)
	}()
	for a.idleFor(time.Now()) != 0 { // the stream is in flight
		time.Sleep(time.Millisecond)
	}
	if got := a.idleFor(time.Now().Add(time.Hour)); got != 0 {
		t.Fatalf("idle %v while a stream is open", got)
	}
	close(release)
	<-done
	if got := a.idleFor(time.Now().Add(time.Hour)); got < 59*time.Minute {
		t.Fatalf("idle %v after the stream ended", got)
	}
}

func TestWatchIdle(t *testing.T) {
	a := newActivity(time.Now().Add(-time.Hour))
	stopped := make(chan struct{})
	go watchIdle(t.Context(), a, 30*time.Minute, time.Millisecond, func() { close(stopped) })
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("not stopped after an idle hour")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	watchIdle(ctx, a, 0, time.Millisecond, func() { t.Fatal("0 means never") })
}

func TestServerIdleEnv(t *testing.T) {
	for v, want := range map[string]time.Duration{"": defaultIdle, "0": 0, "45m": 45 * time.Minute, "nonsense": defaultIdle} {
		t.Setenv("GASCOIGNE_SERVER_IDLE", v)
		if got := serverIdle(); got != want {
			t.Errorf("%q: got %v, want %v", v, got, want)
		}
	}
}

func TestCodexUnwrapRemovesEveryHook(t *testing.T) {
	text := "model = \"o3\"\n\n" + codexTOML("gascoigne")
	got, what := removeCodexEntries(text)
	if strings.Contains(got, "gascoigne") || len(what) != 3 {
		t.Fatalf("left %q, removed %v", got, what)
	}
}
