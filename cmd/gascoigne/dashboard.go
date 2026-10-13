package main

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
)

//go:embed dashboard.html
var dashboardHTML []byte

//go:embed mcp.html
var mcpHTML []byte

//go:embed context.html
var contextHTML []byte

//go:embed pages.css
var pagesCSS []byte

//go:embed pages.js
var pagesJS []byte

// cmdDashboard makes sure the background server runs, prints its address and
// opens it in a browser.
func cmdDashboard(ctx context.Context, args []string) error {
	return cmdServer(ctx, append([]string{"start", "-open"}, args...))
}

// dashboard routes the page and its JSON API; poll is how often the live
// stream checks the ledger, which other gascoigne processes write to. stop
// ends the server.
func dashboard(l *ledger.Ledger, poll time.Duration, info serverInfo, stop func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, info, nil)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		go stop() // after the response is written
	})
	mux.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		q, err := statsQuery(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n, err := l.Reset(r.Context(), q, r.URL.Query().Get("all") == "1")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respond(w, map[string]int64{"deleted_events": n}, nil)
	})
	mux.HandleFunc("GET /api/originals", func(w http.ResponseWriter, r *http.Request) {
		u, err := l.RawUsage(r.Context())
		respond(w, u, err)
	})
	mux.HandleFunc("POST /api/flush", func(w http.ResponseWriter, r *http.Request) {
		n, err := l.FlushRaw(r.Context(), time.Time{})
		respond(w, map[string]int64{"deleted_originals": n}, err)
	})
	setupHandlers(mux)
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(mcpHTML)
	})
	mux.HandleFunc("GET /assets/pages.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(pagesCSS)
	})
	mux.HandleFunc("GET /assets/pages.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(pagesJS)
	})
	mux.HandleFunc("GET /context", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(contextHTML)
	})
	mux.HandleFunc("GET /api/context", func(w http.ResponseWriter, r *http.Request) {
		period, err := parseSince(cmp.Or(r.URL.Query().Get("since"), "14d"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		report, err := loadContext(claudeProjects(), min(max(period, time.Hour), 90*24*time.Hour), time.Now())
		respond(w, report, err)
	})
	mux.HandleFunc("GET /api/mcp", func(w http.ResponseWriter, r *http.Request) {
		review, err := loadReview(r.Context(), l, time.Now(), lazyChoices())
		respond(w, review, err)
	})
	mux.HandleFunc("POST /api/mcp/lazy", setLazy(l))
	newLoginFlows(authStore(), tryOpenBrowser).handlers(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(dashboardHTML)
	})
	mux.HandleFunc("GET /api/overview", func(w http.ResponseWriter, r *http.Request) {
		q, err := statsQuery(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		o, err := mcptool.BuildOverview(r.Context(), l, q)
		respond(w, o, err)
	})
	mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, r *http.Request) {
		q, err := statsQuery(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		models, err := l.UsageStats(r.Context(), q)
		if err != nil {
			respond(w, nil, err)
			return
		}
		days, err := l.UsageDays(r.Context(), q)
		respond(w, map[string]any{"models": nonNil(models), "days": nonNil(days)}, err)
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		stats, err := l.Stats(r.Context(), component.StatsQuery{By: component.BySession})
		respond(w, mcptool.Report(component.StatsQuery{By: component.BySession}, stats).Groups, err)
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		q, err := statsQuery(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		v := r.URL.Query()
		after, _ := strconv.ParseUint(v.Get("after"), 10, strconv.IntSize)
		before, _ := strconv.ParseUint(v.Get("before"), 10, strconv.IntSize)
		limit, _ := strconv.Atoi(v.Get("limit"))
		entries, err := l.Page(r.Context(), q, ledger.Page{After: uint(after), Before: uint(before),
			Outcomes: outcomes(v), Limit: min(cmp.Or(limit, 100), 500)})
		respond(w, eventsJSON(entries), err)
	})
	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		q, err := statsQuery(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		stream(w, r, l, q, outcomes(r.URL.Query()), poll)
	})
	mux.HandleFunc("GET /api/raw/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Unlike gascoigne raw, looking does not mark the source as failing:
		// a person debugging is not an agent that lacked information.
		key, p, err := l.GetRaw(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "original not found (expired or unknown id)", http.StatusNotFound)
			return
		}
		if hiddenKey(key) {
			http.Error(w, hiddenText, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, p.Text())
	})
	mux.HandleFunc("GET /api/out/{id}", func(w http.ResponseWriter, r *http.Request) {
		if key, _, err := l.GetRaw(r.Context(), r.PathValue("id")); err == nil && hiddenKey(key) {
			http.Error(w, hiddenText, http.StatusForbidden)
			return
		}
		out, ok, err := l.GetOutput(r.Context(), r.PathValue("id"))
		if err != nil || !ok {
			http.Error(w, "optimized output not kept for this run", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, out)
	})
	mux.HandleFunc("GET /api/failures", func(w http.ResponseWriter, r *http.Request) {
		fs, err := l.FailureList(r.Context())
		type failure struct {
			Key    string    `json:"key"`
			Count  int       `json:"count"`
			Last   time.Time `json:"last"`
			Reason string    `json:"reason"`
		}
		out := make([]failure, len(fs))
		for i, f := range fs {
			out[i] = failure{f.Key, f.Count, f.Last, f.Reason}
		}
		respond(w, out, err)
	})
	mux.HandleFunc("POST /api/forget", func(w http.ResponseWriter, r *http.Request) {
		if err := l.Forget(r.Context(), r.URL.Query().Get("key")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return localOnly(mux)
}

// loopbackHost reports whether hostport names this machine: localhost or a
// loopback IP, with or without a port.
func loopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

// localOnly refuses requests addressed to another host name (DNS rebinding)
// and cross-origin writes, so only pages served here can use the API.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "dashboard answers on localhost only", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); r.Method != http.MethodGet && o != "" && o != "http://"+r.Host {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// stream sends the latest events, then each new batch as it lands, as
// server-sent events.
func stream(w http.ResponseWriter, r *http.Request, l *ledger.Ledger, q component.StatsQuery, only []string, poll time.Duration) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	var last uint
	send := func(limit int) error {
		entries, err := l.Page(r.Context(), q, ledger.Page{After: last, Outcomes: only, Limit: limit})
		if err != nil || len(entries) == 0 {
			return err
		}
		last = entries[0].ID
		data, err := json.Marshal(eventsJSON(entries))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return nil
	}
	if err := send(50); err != nil { // older runs load on scroll (/api/events?before=)
		return
	}
	io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if err := send(500); err != nil {
				return
			}
		}
	}
}

// outcomes reads ?outcome=optimized,bypassed for the run feed.
func outcomes(v url.Values) []string { return splitList(v.Get("outcome"), ",") }

func statsQuery(v url.Values) (component.StatsQuery, error) {
	q := component.StatsQuery{Session: v.Get("session"), Agent: v.Get("agent"), Model: v.Get("model")}
	if s := v.Get("since"); s != "" {
		d, err := parseSince(s)
		if err != nil {
			return q, err
		}
		q.Since = time.Now().Add(-d)
	}
	return q, nil
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

type stepJSON struct {
	Optimizer string `json:"optimizer"`
	Family    string `json:"family"`
	Dialect   string `json:"dialect"`
	RawTokens int    `json:"raw_tokens"`
	OutTokens int    `json:"out_tokens"`
}

type eventJSON struct {
	ID         uint       `json:"id"`
	TS         time.Time  `json:"ts"`
	Session    string     `json:"session"`
	Agent      string     `json:"agent"`
	Model      string     `json:"model"`
	Command    string     `json:"command"`
	Pipeline   string     `json:"pipeline"`
	Key        string     `json:"key"`
	Route      string     `json:"route"`
	Optimizers []string   `json:"optimizers"`
	Outcome    string     `json:"outcome"`
	Filter     string     `json:"filter"`
	Reason     string     `json:"reason"`
	RawTokens  int        `json:"raw_tokens"`
	OutTokens  int        `json:"out_tokens"`
	DurationMS int64      `json:"duration_ms"`
	RawID      string     `json:"raw_id"`
	Steps      []stepJSON `json:"steps"`
}

func eventsJSON(entries []ledger.Entry) []eventJSON {
	out := make([]eventJSON, len(entries))
	for i, e := range entries {
		steps := make([]stepJSON, len(e.Steps))
		for j, s := range e.Steps {
			steps[j] = stepJSON{s.Optimizer, s.Family, s.Dialect, s.RawTokens, s.OutTokens}
		}
		out[i] = eventJSON{
			ID: e.ID, TS: e.TS, Session: e.Session, Agent: e.Agent, Model: e.Model, Command: e.Command,
			Pipeline: e.Pipeline, Key: e.Key, Route: e.Route, Optimizers: e.Optimizers, Outcome: string(e.Outcome),
			Filter: e.Filter, Reason: e.Reason, RawTokens: e.RawTokens, OutTokens: e.OutTokens,
			DurationMS: e.Duration.Milliseconds(), RawID: e.RawID, Steps: steps,
		}
	}
	return out
}

// Instruction text (system prompt, CLAUDE.md, skill list, reminders) is
// recorded for its size; its content stays out of the dashboard, which
// anyone with the page open could read. gascoigne raw <id> still prints it.
const hiddenText = "instruction text is not shown in the dashboard; gascoigne raw <id> prints it"

func hiddenKey(key string) bool { return strings.HasPrefix(key, "prompt:") }

// nonNil turns a nil slice into an empty one, so JSON holds [] and a page can
// read its length.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
