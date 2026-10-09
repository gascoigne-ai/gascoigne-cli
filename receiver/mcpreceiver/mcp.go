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

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/mcptool"
	"github.com/lukaszb/gascoigne/payload"
)

const version = "0.1.0"

type Receiver struct {
	cfg       *Config
	next      component.Pipeline
	log       *slog.Logger
	ledger    component.Ledger
	transport mcp.Transport
	http      *http.Client
}

func (r *Receiver) Start(_ context.Context, host component.Host) error {
	l, err := mcptool.Ledger(host, r.cfg.Ledger)
	if err != nil {
		return err
	}
	r.ledger = l
	return nil
}

func (r *Receiver) Shutdown(context.Context) error { return nil }

func (r *Receiver) Run(ctx context.Context) error {
	return r.server().Run(ctx, r.transport)
}

func (r *Receiver) server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "gascoigne", Version: version}, &mcp.ServerOptions{
		Instructions: "Use fetch for web pages and HTTP APIs, compress for large text you already have. " +
			"Results may be optimized; when a detail is missing call " + r.cfg.Retrieve + " with the id from the hint.",
		Logger: r.log,
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "fetch",
		Description: "GET a URL and return its content compactly: HTML as markdown, JSON compressed. raw=true skips optimization.",
	}, r.fetch)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "compress",
		Description: "Compress text (command output, logs, JSON, code) to fewer tokens. cmd/file hints improve detection.",
	}, r.compress)
	mcptool.AddRetrieve(s, r.cfg.Retrieve, r.ledger)
	mcptool.AddStats(s, r.ledger)
	return s
}

type FetchInput struct {
	URL string `json:"url" jsonschema:"http or https URL"`
	Raw bool   `json:"raw,omitempty" jsonschema:"return the body unoptimized"`
}

func (r *Receiver) fetch(ctx context.Context, _ *mcp.CallToolRequest, in FetchInput) (*mcp.CallToolResult, any, error) {
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, nil, fmt.Errorf("url must be http or https: %q", in.URL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "gascoigne/"+version)
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, r.cfg.FetchMaxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	truncated := int64(len(body)) > r.cfg.FetchMaxBytes
	if truncated {
		body = body[:r.cfg.FetchMaxBytes]
	}
	p := payload.New(payload.ToolResult, string(body))
	p.Set(payload.AttrURL, u.String())
	p.Set(payload.AttrTool, "fetch")
	p.Set("mime", resp.Header.Get("Content-Type"))
	text, err := r.process(ctx, p, in.Raw)
	if err != nil {
		return nil, nil, err
	}
	head := fmt.Sprintf("HTTP %d %s\n", resp.StatusCode, u)
	if truncated {
		head += fmt.Sprintf("[body truncated at %d bytes]\n", r.cfg.FetchMaxBytes)
	}
	return mcptool.Result(head + text), nil, nil
}

type CompressInput struct {
	Text string `json:"text"`
	Cmd  string `json:"cmd,omitempty" jsonschema:"command that produced the text, e.g. go test ./..."`
	File string `json:"file,omitempty" jsonschema:"file the text came from"`
	Raw  bool   `json:"raw,omitempty" jsonschema:"return text unchanged"`
}

func (r *Receiver) compress(ctx context.Context, _ *mcp.CallToolRequest, in CompressInput) (*mcp.CallToolResult, any, error) {
	if in.Text == "" {
		return nil, nil, errors.New("text is empty")
	}
	p := payload.New(payload.ToolResult, in.Text)
	p.Set(payload.AttrCmd, in.Cmd)
	p.Set(payload.AttrFile, in.File)
	text, err := r.process(ctx, p, in.Raw)
	if err != nil {
		return nil, nil, err
	}
	return mcptool.Result(text), nil, nil
}

func (r *Receiver) process(ctx context.Context, p *payload.Payload, raw bool) (string, error) {
	if raw {
		p.Set(payload.AttrRaw, "true")
	}
	out, err := r.next.Process(ctx, p)
	if err != nil {
		return "", err
	}
	return mcptool.Text(out, r.cfg.Retrieve), nil
}
