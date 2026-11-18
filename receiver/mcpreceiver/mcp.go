// Package mcpreceiver is an MCP server (stdio) exposing the pipeline to
// agents: fetch web pages and APIs as compact markdown/JSON, compress any
// text, retrieve originals and report savings.
package mcpreceiver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Receiver struct {
	cfg       *Config
	next      component.Pipeline
	log       *slog.Logger
	ledger    component.Ledger
	transport mcp.Transport
	http      *http.Client
	sessions  *mcptool.Sessions
	fetched   *fetchCache
}

func (r *Receiver) Start(_ context.Context, host component.Host) error {
	l, err := mcptool.Ledger(host, mcptool.LedgerID)
	if err != nil {
		return err
	}
	r.ledger = l
	return nil
}

func (r *Receiver) Shutdown(context.Context) error { return nil }

func (r *Receiver) Run(ctx context.Context) error {
	r.sessions = mcptool.NewSessions(ctx)
	return r.server().Run(ctx, r.transport)
}

func (r *Receiver) server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "gascoigne", Version: version.Number}, &mcp.ServerOptions{
		Instructions: "Use fetch for web pages and HTTP APIs, compress for large text you already have. " +
			"Results may be optimized; when a detail is missing call " + r.cfg.Retrieve + " with the id from the hint.",
		Logger: r.log,
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "fetch",
		Description: "GET a URL and return its content compactly: HTML as markdown, JSON compressed. A repeat in the session returns unchanged or a diff. raw=true skips optimization.",
	}, r.fetch)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "compress",
		Description: "Compress text (command output, logs, JSON, code) to fewer tokens. cmd/file hints improve detection.",
	}, r.compress)
	mcptool.AddShow(s)
	mcptool.AddRetrieve(s, r.cfg.Retrieve, r.ledger)
	mcptool.AddStats(s, r.ledger, r.sessions)
	return s
}

type FetchInput struct {
	URL   string `json:"url" jsonschema:"http or https URL"`
	Query string `json:"query,omitempty" jsonschema:"what you are looking for; matching items and page sections survive compression"`
	Raw   bool   `json:"raw,omitempty" jsonschema:"return the body unoptimized"`
}

func (r *Receiver) fetch(ctx context.Context, call *mcp.CallToolRequest, in FetchInput) (*mcp.CallToolResult, any, error) {
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, nil, fmt.Errorf("url must be http or https: %q", in.URL)
	}
	page, err := r.download(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	head := fmt.Sprintf("HTTP %d %s\n", page.status, u)
	if page.truncated {
		head += fmt.Sprintf("[body truncated at %d bytes]\n", r.cfg.FetchMaxBytes)
	}
	p := payload.New(payload.ToolResult, string(page.body))
	p.Set(payload.AttrURL, u.String())
	p.Set(payload.AttrTool, "fetch")
	p.Set("mime", page.mime)
	p.Set(payload.AttrQuery, in.Query)
	r.sessions.Tag(p, call)
	if in.Raw {
		text, err := r.process(ctx, p, true)
		if err != nil {
			return nil, nil, err
		}
		return mcptool.Result(head + text), nil, nil
	}
	text, err := r.fetchOptimized(ctx, p, head, page, u.String(), in.Query)
	if err != nil {
		return nil, nil, err
	}
	return mcptool.Result(text), nil, nil
}

// fetchedPage is a downloaded page: the status line facts and the (possibly cut) body.
type fetchedPage struct {
	status    int
	mime      string
	body      []byte
	truncated bool
}

// download GETs target, reading at most FetchMaxBytes of the body.
func (r *Receiver) download(ctx context.Context, target *url.URL) (fetchedPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchedPage{}, err
	}
	req.Header.Set("User-Agent", "gascoigne/"+version.Number)
	resp, err := r.http.Do(req)
	if err != nil {
		return fetchedPage{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, r.cfg.FetchMaxBytes+1))
	if err != nil {
		return fetchedPage{}, err
	}
	page := fetchedPage{status: resp.StatusCode, mime: resp.Header.Get("Content-Type"), body: body}
	if int64(len(body)) > r.cfg.FetchMaxBytes {
		page.truncated = true
		page.body = body[:r.cfg.FetchMaxBytes]
	}
	return page, nil
}

// fetchOptimized runs the pipeline, answering with a pointer when the page is unchanged since the
// session's earlier fetch and with a line diff when it changed.
func (r *Receiver) fetchOptimized(ctx context.Context, request *payload.Payload, head string, page fetchedPage, rawURL, query string) (string, error) {
	key := fetchKey(request.Attrs[payload.AttrSession], rawURL, query)
	hash := fetchHash(page.status, page.body)
	prev, seen := r.fetched.Get(key)
	if seen && prev.hash == hash {
		return fmt.Sprintf("%sunchanged since earlier fetch in this session · full: %s id=%s", head, r.cfg.Retrieve, prev.rawID), nil
	}
	out, err := r.next.Process(ctx, request)
	if err != nil {
		return "", err
	}
	// Only optimized results are cached: they carry the id a pointer needs.
	// A bypassed one (failed key, skip) leaves the entry alone, so it never
	// turns later fetches into permanent full results.
	id := out.Attrs[payload.AttrRawID]
	if id == "" {
		return head + mcptool.Text(out), nil
	}
	r.fetched.Put(key, fetchEntry{hash: hash, rawID: id, text: out.Text()})
	if seen {
		if d, ok := lineDiff(prev.text, out.Text()); ok {
			return fmt.Sprintf("%schanged since earlier fetch in this session (full: %s id=%s):\n%s", head, r.cfg.Retrieve, id, d), nil
		}
	}
	return head + mcptool.Text(out), nil
}

type CompressInput struct {
	Text  string `json:"text"`
	Cmd   string `json:"cmd,omitempty" jsonschema:"command that produced the text, e.g. go test ./..."`
	File  string `json:"file,omitempty" jsonschema:"file the text came from"`
	Query string `json:"query,omitempty" jsonschema:"what you are looking for; matching items and page sections survive compression"`
	Raw   bool   `json:"raw,omitempty" jsonschema:"return text unchanged"`
}

func (r *Receiver) compress(ctx context.Context, req *mcp.CallToolRequest, in CompressInput) (*mcp.CallToolResult, any, error) {
	if in.Text == "" {
		return nil, nil, errors.New("text is empty")
	}
	p := payload.New(payload.ToolResult, in.Text)
	p.Set(payload.AttrCmd, in.Cmd)
	p.Set(payload.AttrFile, in.File)
	p.Set(payload.AttrQuery, in.Query)
	r.sessions.Tag(p, req)
	text, err := r.process(ctx, p, in.Raw)
	if err != nil {
		return nil, nil, err
	}
	return mcptool.Result(text), nil, nil
}

func (r *Receiver) process(ctx context.Context, data *payload.Payload, raw bool) (string, error) {
	if raw {
		data.Set(payload.AttrRaw, "true")
	}
	out, err := r.next.Process(ctx, data)
	if err != nil {
		return "", err
	}
	return mcptool.Text(out), nil
}
