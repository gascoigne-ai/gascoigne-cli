package main

import (
	"cmp"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// staticPage is one embedded file the dashboard serves as it is.
type staticPage struct {
	pattern     string
	contentType string
	noCache     bool // assets change with the binary, pages are fetched fresh anyway
	body        []byte
}

const (
	htmlType = "text/html; charset=utf-8"
	textType = "text/plain; charset=utf-8"
)

// staticPages are the pages and assets of the dashboard (the setup page is
// served by setupHandlers).
func staticPages() []staticPage {
	return []staticPage{
		{"GET /mcp", htmlType, false, mcpHTML},
		{"GET /costs", htmlType, false, costsHTML},
		{"GET /context", htmlType, false, contextHTML},
		{"GET /{$}", htmlType, false, dashboardHTML},
		{"GET /assets/pages.css", "text/css; charset=utf-8", true, pagesCSS},
		{"GET /assets/pages.js", "text/javascript; charset=utf-8", true, pagesJS},
	}
}

// assetHandlers serves staticPages.
func assetHandlers(mux *http.ServeMux) {
	for _, page := range staticPages() {
		mux.HandleFunc(page.pattern, func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", page.contentType)
			if page.noCache {
				writer.Header().Set("Cache-Control", "no-cache")
			}
			writer.Write(page.body)
		})
	}
}

// serverHandlers answer about and control the server itself.
func serverHandlers(mux *http.ServeMux, info serverInfo, stop func()) {
	mux.HandleFunc("GET /api/health", func(writer http.ResponseWriter, _ *http.Request) {
		respond(writer, info, nil)
	})
	mux.HandleFunc("GET /api/experiments", func(writer http.ResponseWriter, _ *http.Request) {
		respond(writer, enabledExperiments(), nil)
	})
	mux.HandleFunc("POST /api/shutdown", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusAccepted)
		go stop() // after the response is written
	})
}

// ledgerHandlers change or list what the ledger holds.
func ledgerHandlers(mux *http.ServeMux, store *ledger.Ledger) {
	mux.HandleFunc("POST /api/reset", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		deleted, err := store.Reset(request.Context(), query, request.URL.Query().Get("all") == "1")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		respond(writer, map[string]int64{"deleted_events": deleted}, nil)
	})
	mux.HandleFunc("GET /api/originals", func(writer http.ResponseWriter, request *http.Request) {
		usage, err := store.RawUsage(request.Context())
		respond(writer, usage, err)
	})
	mux.HandleFunc("POST /api/flush", func(writer http.ResponseWriter, request *http.Request) {
		deleted, err := store.FlushRaw(request.Context(), time.Time{})
		respond(writer, map[string]int64{"deleted_originals": deleted}, err)
	})
	mux.HandleFunc("GET /api/failures", func(writer http.ResponseWriter, request *http.Request) {
		failures, err := store.FailureList(request.Context())
		type failure struct {
			Key    string    `json:"key"`
			Count  int       `json:"count"`
			Last   time.Time `json:"last"`
			Reason string    `json:"reason"`
		}
		out := make([]failure, len(failures))
		for i, entry := range failures {
			out[i] = failure{entry.Key, entry.Count, entry.Last, entry.Reason}
		}
		respond(writer, out, err)
	})
	mux.HandleFunc("POST /api/forget", func(writer http.ResponseWriter, request *http.Request) {
		if err := store.Forget(request.Context(), request.URL.Query().Get("key")); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

// costHandlers report what the models cost and how much of it the runs used.
func costHandlers(mux *http.ServeMux, store *ledger.Ledger) {
	mux.HandleFunc("GET /api/costs", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		costs, err := mcptool.BuildCosts(request.Context(), store, query, cost.AgentContext)
		respond(writer, costs, err)
	})
	mux.HandleFunc("GET /api/usage", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		models, err := store.UsageStats(request.Context(), query)
		if err != nil {
			respond(writer, nil, err)
			return
		}
		days, err := store.UsageDays(request.Context(), query)
		respond(writer, map[string]any{"models": nonNil(models), "days": nonNil(days)}, err)
	})
}

// reportHandlers are the other JSON reports the pages draw: one stats query in, one
// report out.
func reportHandlers(mux *http.ServeMux, store *ledger.Ledger) {
	mux.HandleFunc("GET /api/context", func(writer http.ResponseWriter, request *http.Request) {
		period, err := parseSince(cmp.Or(request.URL.Query().Get("since"), "14d"))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		report, err := loadContext(claudeProjects(), min(max(period, time.Hour), 90*24*time.Hour), time.Now())
		respond(writer, report, err)
	})
	mux.HandleFunc("GET /api/overview", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		overview, err := mcptool.BuildOverview(request.Context(), store, query)
		respond(writer, overview, err)
	})
	mux.HandleFunc("GET /api/impact", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		days, err := store.Impact(request.Context(), query)
		respond(writer, nonNil(fillImpact(days, query.Since, time.Now())), err)
	})
	mux.HandleFunc("GET /api/sessions", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		query.Session, query.By = "", component.BySession // the list offers every session of the period
		stats, err := store.Stats(request.Context(), query)
		respond(writer, mcptool.Report(query, stats).Groups, err)
	})
}

// runHandlers list the runs, live, and show what one run read and returned.
func runHandlers(mux *http.ServeMux, store *ledger.Ledger, poll time.Duration) {
	mux.HandleFunc("GET /api/events", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		values := request.URL.Query()
		after, _ := strconv.ParseUint(values.Get("after"), 10, strconv.IntSize)
		before, _ := strconv.ParseUint(values.Get("before"), 10, strconv.IntSize)
		limit, _ := strconv.Atoi(values.Get("limit"))
		entries, err := store.Page(request.Context(), query, ledger.Page{After: uint(after), Before: uint(before),
			Outcomes: outcomes(values), RawAsked: values.Get("raw") == "1", Limit: min(cmp.Or(limit, 100), 500)})
		respond(writer, eventsJSON(entries), err)
	})
	mux.HandleFunc("GET /api/stream", func(writer http.ResponseWriter, request *http.Request) {
		query, err := statsQuery(request.URL.Query())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		stream(writer, request, store, query, ledger.Page{Outcomes: outcomes(request.URL.Query()), RawAsked: request.URL.Query().Get("raw") == "1"}, poll)
	})
	mux.HandleFunc("GET /api/raw/{id}", func(writer http.ResponseWriter, request *http.Request) {
		// Unlike gascoigne raw, looking does not mark the source as failing:
		// a person debugging is not an agent that lacked information.
		key, original, err := store.GetRaw(request.Context(), request.PathValue("id"))
		if err != nil {
			http.Error(writer, "original not found (expired or unknown id)", http.StatusNotFound)
			return
		}
		if hiddenKey(key) {
			http.Error(writer, hiddenText, http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", textType)
		io.WriteString(writer, original.Text())
	})
	mux.HandleFunc("GET /api/out/{id}", func(writer http.ResponseWriter, request *http.Request) {
		if key, _, err := store.GetRaw(request.Context(), request.PathValue("id")); err == nil && hiddenKey(key) {
			http.Error(writer, hiddenText, http.StatusForbidden)
			return
		}
		out, ok, err := store.GetOutput(request.Context(), request.PathValue("id"))
		if err != nil || !ok {
			http.Error(writer, "optimized output not kept for this run", http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", textType)
		io.WriteString(writer, out)
	})
}

// mcpHandlers are the MCP review page's calls and the sign-in flows.
func mcpHandlers(mux *http.ServeMux, store *ledger.Ledger) {
	mux.HandleFunc("GET /api/mcp", func(writer http.ResponseWriter, request *http.Request) {
		review, err := loadReview(request.Context(), store, time.Now(), lazyChoices(), readPrefs().LearnedMCP)
		respond(writer, review, err)
	})
	mux.HandleFunc("POST /api/mcp/lazy", setLazy(store))
	mux.HandleFunc("POST /api/mcp/learn", learnNow(store))
	mux.HandleFunc("POST /api/mcp/rules", setRules(store))
	mux.HandleFunc("POST /api/mcp/optimize", optimizeMCP(store))
	newLoginFlows(authStore(), tryOpenBrowser).handlers(mux)
}
