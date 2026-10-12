package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/mcpauth"
)

// The OAuth logins of remote MCP servers run in the background server, not in
// the mcp-proxy process: an agent starts the proxy, often with no display, in
// a sandbox, and stops it when it likes. The server is gascoigne's own,
// outlives the agent and opens the browser itself.

// loginState is what the server knows of one login.
type loginState struct {
	State   string `json:"state"` // running, done or failed
	AuthURL string `json:"auth_url,omitempty"`
	Error   string `json:"error,omitempty"`
}

// loginFlows are the logins the server runs, by server url.
type loginFlows struct {
	open  func(authURL string)
	store *mcpauth.Store
	mu    sync.Mutex
	byURL map[string]*loginState
}

func newLoginFlows(store *mcpauth.Store, open func(string)) *loginFlows {
	return &loginFlows{open: open, store: store, byURL: map[string]*loginState{}}
}

// start begins a login unless one runs for the server; a finished one is replaced.
func (f *loginFlows) start(serverURL string) loginState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st := f.byURL[serverURL]; st != nil && st.State == "running" {
		return *st
	}
	st := &loginState{State: "running"}
	f.byURL[serverURL] = st
	go func() {
		err := mcpauth.Login(context.Background(), f.store, serverURL, func(authURL string) error {
			f.mu.Lock()
			st.AuthURL = authURL
			f.mu.Unlock()
			f.open(authURL)
			return nil
		})
		f.mu.Lock()
		defer f.mu.Unlock()
		if st.State, st.Error = "done", ""; err != nil {
			st.State, st.Error = "failed", err.Error()
		}
	}()
	return *st
}

func (f *loginFlows) status(serverURL string) (loginState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st := f.byURL[serverURL]; st != nil {
		return *st, true
	}
	return loginState{}, false
}

// handlers mounts POST /api/mcp/login {url} (start) and GET /api/mcp/login?url= (state).
func (f *loginFlows) handlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/mcp/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil || !httpURL(in.URL) {
			http.Error(w, "want {url: an http(s) url}", http.StatusBadRequest)
			return
		}
		respond(w, f.start(in.URL), nil)
	})
	mux.HandleFunc("GET /api/mcp/login", func(w http.ResponseWriter, r *http.Request) {
		st, ok := f.status(r.URL.Query().Get("url"))
		if !ok {
			http.Error(w, "no login for this url", http.StatusNotFound)
			return
		}
		respond(w, st, nil)
	})
}

func httpURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// remoteLogin is how mcp-proxy logs in: through the background server, which
// opens the browser; in this process only when no server answers.
func remoteLogin(ctx context.Context, serverURL string, shown func(authURL string)) error {
	err := loginViaServer(ctx, serverAddr(), serverURL, shown)
	if !errors.Is(err, errUnavailable) {
		return err
	}
	return mcpauth.Login(ctx, authStore(), serverURL, func(authURL string) error {
		shown(authURL)
		openBrowser(authURL)
		return nil
	})
}

// loginViaServer asks the server at addr to run the login and waits for it.
func loginViaServer(ctx context.Context, addr, serverURL string, shown func(string)) error {
	body, _ := json.Marshal(map[string]string{"url": serverURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/mcp/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	st, err := loginRequest(req)
	if err != nil {
		return err
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	sentURL := false
	for {
		if st.AuthURL != "" && !sentURL {
			shown(st.AuthURL)
			sentURL = true
		}
		switch st.State {
		case "done":
			return nil
		case "failed":
			return fmt.Errorf("login: %s", st.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
		poll, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/mcp/login?url="+url.QueryEscape(serverURL), nil)
		if err != nil {
			return err
		}
		if st, err = loginRequest(poll); err != nil {
			return err
		}
	}
}

func loginRequest(req *http.Request) (loginState, error) {
	var st loginState
	resp, err := remoteClient.Do(req)
	if err != nil {
		return st, fmt.Errorf("%w: %v", errUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st, fmt.Errorf("%w: login: %s", errUnavailable, resp.Status)
	}
	return st, json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<16)).Decode(&st)
}
