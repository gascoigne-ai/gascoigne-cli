package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"
)

// defaultIdle is how long the background server waits without a request
// before it exits; the next command or hook that needs it starts a new one.
const defaultIdle = 30 * time.Minute

// serverIdle is the idle timeout from $GASCOIGNE_SERVER_IDLE ("45m", "0"
// for never), else defaultIdle.
func serverIdle() time.Duration {
	if v := os.Getenv("GASCOIGNE_SERVER_IDLE"); v != "" {
		if v == "0" {
			return 0
		}
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			return d
		}
	}
	return defaultIdle
}

// activity counts requests in flight and remembers when the last one
// ended. An open stream (the dashboard's live feed, an agent's response)
// is activity for as long as it lasts.
type activity struct {
	mu       sync.Mutex
	inFlight int
	last     time.Time
}

func newActivity(now time.Time) *activity { return &activity{last: now} }

func (a *activity) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" { // a liveness check is not use
			next.ServeHTTP(w, r)
			return
		}
		a.change(1)
		defer a.change(-1)
		next.ServeHTTP(w, r)
	})
}

func (a *activity) change(d int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inFlight += d
	a.last = time.Now()
}

// idleFor is how long nothing has been in flight; zero while something is.
func (a *activity) idleFor(now time.Time) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inFlight > 0 {
		return 0
	}
	return now.Sub(a.last)
}

// watchIdle calls stop once the server has been idle for idle, checking
// every tick. Zero idle never stops.
func watchIdle(ctx context.Context, a *activity, idle, tick time.Duration, stop func()) {
	if idle <= 0 {
		return
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if a.idleFor(now) >= idle {
				stop()
				return
			}
		}
	}
}
