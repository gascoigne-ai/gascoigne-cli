package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

//go:embed dashboard.html
var dashboardHTML []byte

//go:embed mcp.html
var mcpHTML []byte

//go:embed costs.html
var costsHTML []byte

//go:embed context.html
var contextHTML []byte

//go:embed pages.css
var pagesCSS []byte

//go:embed pages.js
var pagesJS []byte

// cmdDashboard makes sure the background server runs, prints its address and
// opens it in a browser.
func cmdDashboard(ctx context.Context, args []string) error {
	checkReleaseQuick(ctx)
	if note := releaseNudge("dashboard"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	return cmdServer(ctx, append([]string{"start", "-open"}, args...))
}

// dashboard routes the page and its JSON API; poll is how often the live
// stream checks the ledger, which other gascoigne processes write to. stop
// ends the server.
func dashboard(store *ledger.Ledger, poll time.Duration, info serverInfo, stop func()) http.Handler {
	mux := http.NewServeMux()
	serverHandlers(mux, info, stop)
	ledgerHandlers(mux, store)
	setupHandlers(mux)
	pricingHandlers(mux)
	assetHandlers(mux)
	costHandlers(mux, store)
	reportHandlers(mux, store)
	runHandlers(mux, store, poll)
	mcpHandlers(mux, store)
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
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !loopbackHost(request.Host) {
			http.Error(writer, "dashboard answers on localhost only", http.StatusForbidden)
			return
		}
		if o := request.Header.Get("Origin"); request.Method != http.MethodGet && o != "" && o != "http://"+request.Host {
			http.Error(writer, "cross-origin request refused", http.StatusForbidden)
			return
		}
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, request)
	})
}

// stream sends the latest events, then each new batch as it lands, as
// server-sent events.
func stream(writer http.ResponseWriter, request *http.Request, store *ledger.Ledger, query component.StatsQuery, feed ledger.Page, poll time.Duration) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	var last uint
	send := func(limit int) error {
		entries, err := store.Page(request.Context(), query, ledger.Page{After: last, Outcomes: feed.Outcomes, RawAsked: feed.RawAsked, Limit: limit})
		if err != nil || len(entries) == 0 {
			return err
		}
		last = entries[0].ID
		data, err := json.Marshal(eventsJSON(entries))
		if err != nil {
			return err
		}
		fmt.Fprintf(writer, "data: %s\n\n", data)
		flusher.Flush()
		return nil
	}
	if err := send(50); err != nil { // older runs load on scroll (/api/events?before=)
		return
	}
	io.WriteString(writer, ": connected\n\n")
	flusher.Flush()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-tick.C:
			if err := send(500); err != nil {
				return
			}
		}
	}
}

// outcomes reads ?outcome=optimized,bypassed for the run feed (?raw=1 keeps
// runs where the agent asked for the original at once).
func outcomes(values url.Values) []string { return splitList(values.Get("outcome"), ",") }

func statsQuery(values url.Values) (component.StatsQuery, error) {
	q := component.StatsQuery{Session: values.Get("session"), Agent: values.Get("agent"), Model: values.Get("model")}
	if s := values.Get("since"); s != "" {
		d, err := parseSince(s)
		if err != nil {
			return q, err
		}
		q.Since = time.Now().Add(-d)
	}
	return q, nil
}

// fillImpact adds the quiet days of the period, as the daily chart does. With
// no start (all time) the period begins at the first day that has data.
func fillImpact(days []ledger.ImpactDay, since, now time.Time) []ledger.ImpactDay {
	if since.IsZero() {
		if len(days) == 0 {
			return days
		}
		first, err := time.ParseInLocation(time.DateOnly, days[0].Day, time.Local)
		if err != nil {
			return days
		}
		since = first
	}
	have := map[string]ledger.ImpactDay{}
	for _, d := range days {
		have[d.Day] = d
	}
	out := []ledger.ImpactDay{}
	for _, key := range mcptool.Days(since, now) {
		d, ok := have[key]
		if !ok {
			d = ledger.ImpactDay{Day: key}
		}
		out = append(out, d)
	}
	return out
}

func respond(writer http.ResponseWriter, value any, err error) {
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(value)
}

type stepJSON struct {
	Optimizer string `json:"optimizer"`
	Family    string `json:"family"`
	Dialect   string `json:"dialect"`
	RawTokens int    `json:"raw_tokens"`
	OutTokens int    `json:"out_tokens"`
}

type eventJSON struct {
	ID         uint              `json:"id"`
	TS         time.Time         `json:"ts"`
	Session    string            `json:"session"`
	Agent      string            `json:"agent"`
	Model      string            `json:"model"`
	Command    string            `json:"command"`
	Pipeline   string            `json:"pipeline"`
	Key        string            `json:"key"`
	Route      string            `json:"route"`
	Optimizers []string          `json:"optimizers"`
	Outcome    component.Outcome `json:"outcome"`
	Filter     string            `json:"filter"`
	Reason     string            `json:"reason"`
	RawTokens  int               `json:"raw_tokens"`
	OutTokens  int               `json:"out_tokens"`
	DurationMS int64             `json:"duration_ms"`
	RawID      string            `json:"raw_id"`
	Steps      []stepJSON        `json:"steps"`
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
			Pipeline: e.Pipeline, Key: e.Key, Route: e.Route, Optimizers: e.Optimizers, Outcome: e.Outcome,
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

// nonNil turns a nil slice into an empty one, so JSON holds [] and a page can
// read its length.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
