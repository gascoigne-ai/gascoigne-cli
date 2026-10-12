package mcpproxyreceiver

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/mcpauth"
	"github.com/gascoigne-ai/gascoigne-cli/version"
)

// loginTool is the one tool a remote server offers until its OAuth login is done.
const loginTool = "gascoigne_login"

// canLogin: the proxy can run the OAuth flow itself for this server.
func canLogin(rm Remote) bool {
	return rm.Store != nil && !rm.SSE && !hasAuthorization(rm.Headers)
}

// loginFlow is one login the agent asked for: the browser flow runs in the
// background (the proxy has no terminal), the tool call returns the address to
// open, and the server's real tools replace the login tool once it is done.
type loginFlow struct {
	mu      sync.Mutex
	running bool
	done    bool
	authURL string
	err     error
	ready   chan struct{} // closed once authURL is known or the flow ended
}

// runLogin serves a stand-in for a remote server that answered 401: instructions
// and a login tool. The agent starts without waiting for a browser, and the
// server's tools show up (tools/list_changed) when the login succeeds.
func (r *Receiver) runLogin(ctx context.Context, client *mcp.Client, rm Remote) error {
	host := rm.URL
	if u, err := url.Parse(rm.URL); err == nil {
		host = u.Host
	}
	s := mcp.NewServer(&mcp.Implementation{Name: host, Version: version.Number}, &mcp.ServerOptions{
		Instructions: fmt.Sprintf("The remote MCP server %s needs an OAuth login before it offers any tools. Call %s and tell the user to authorize in the browser.", mcpauth.Display(rm.URL), loginTool),
		Logger:       r.log,
	})
	f := &loginFlow{}
	s.AddTool(&mcp.Tool{
		Name:        loginTool,
		Description: "Log in to " + host + " (OAuth). Starts the browser authorization and returns its address; the server's tools appear when the user is done.",
		InputSchema: map[string]any{"type": "object"},
	}, func(callCtx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.loginStatus(ctx, callCtx, f, client, s, rm)}}}, nil
	})
	r.log.Warn("remote server needs an OAuth login; the agent is offered the login tool", "server", mcpauth.Display(rm.URL))
	return s.Run(ctx, r.upstream)
}

// loginStatus starts the flow when none runs and says where the user stands.
// ctx is the proxy's, which the flow outlives the call with; callCtx is the call's.
func (r *Receiver) loginStatus(ctx, callCtx context.Context, f *loginFlow, client *mcp.Client, s *mcp.Server, rm Remote) string {
	f.mu.Lock()
	if !f.running && !f.done {
		f.running, f.err, f.authURL, f.ready = true, nil, "", make(chan struct{})
		go r.login(ctx, f, f.ready, client, s, rm)
	}
	ready := f.ready
	f.mu.Unlock()

	select {
	case <-ready:
	case <-time.After(15 * time.Second):
	case <-callCtx.Done():
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.done:
		return "Logged in. List the tools again."
	case f.err != nil:
		return "Login failed: " + f.err.Error() + "\nCall " + loginTool + " to try again."
	case f.authURL != "":
		return "Ask the user to authorize in the browser (opened if possible): " + f.authURL + "\nThe server's tools appear when they are done."
	}
	return "Login is starting; call " + loginTool + " again for the address."
}

func (r *Receiver) login(ctx context.Context, f *loginFlow, ready chan struct{}, client *mcp.Client, s *mcp.Server, rm Remote) {
	var once sync.Once
	signal := func() { once.Do(func() { close(ready) }) }
	err := mcpauth.Login(ctx, rm.Store, rm.URL, func(authURL string) error {
		f.mu.Lock()
		f.authURL = authURL
		f.mu.Unlock()
		signal()
		if rm.Open != nil {
			rm.Open(authURL)
		}
		return nil
	})
	if err == nil {
		err = r.finishLogin(ctx, client, s, rm)
	}
	f.mu.Lock()
	f.running, f.done, f.err = false, err == nil, err
	f.mu.Unlock()
	signal()
}

// finishLogin connects with the new login and swaps the login tool for the server's tools.
func (r *Receiver) finishLogin(ctx context.Context, client *mcp.Client, s *mcp.Server, rm Remote) error {
	var unauthorized atomic.Bool
	cs, err := client.Connect(ctx, rm.transport(&unauthorized), nil)
	if err != nil {
		return fmt.Errorf("connect after login: %w", err)
	}
	info := cs.InitializeResult().ServerInfo
	if info == nil {
		info = &mcp.Implementation{Name: "gascoigne-proxy", Version: version.Number}
	}
	if err := r.populate(ctx, s, cs, info, false); err != nil {
		cs.Close()
		return err
	}
	s.RemoveTools(loginTool)
	go func() {
		<-ctx.Done()
		cs.Close()
	}()
	return nil
}
