package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcpauth"
)

// The OAuth logins of remote MCP servers run in the background server, not in
// the mcp-proxy process: an agent starts the proxy, often with no display, in
// a sandbox, and stops it when it likes. The server is gascoigne's own,
// outlives the agent and opens the browser itself.

// loginPhase is where one login stands. Only running moves on, to done or
// failed; a finished login is replaced by a new one, never changed. The
// values are the JSON the proxy reads.
type loginPhase string

const (
	loginRunning loginPhase = "running"
	loginDone    loginPhase = "done"
	loginFailed  loginPhase = "failed"
)

// loginTransitions lists the legal moves; a phase with no row is final.
var loginTransitions = map[loginPhase][]loginPhase{
	loginRunning: {loginDone, loginFailed},
}

// UnmarshalJSON refuses a phase the server never sends.
func (phase *loginPhase) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	next := loginPhase(name)
	if _, known := loginTransitions[next]; !known && next != loginDone && next != loginFailed {
		return fmt.Errorf("unknown login state %q", name)
	}
	*phase = next
	return nil
}

func (phase loginPhase) canMoveTo(next loginPhase) bool {
	return slices.Contains(loginTransitions[phase], next)
}

// loginState is what the server knows of one login.
type loginState struct {
	State   loginPhase `json:"state"`
	AuthURL string     `json:"auth_url,omitempty"`
	Opened  bool       `json:"opened,omitempty"` // a browser was started for AuthURL
	Error   string     `json:"error,omitempty"`
}

// loginFlows are the logins the server runs, by server url.
type loginFlows struct {
	open  func(authURL string) bool
	store *mcpauth.Store
	mu    sync.Mutex
	byURL map[string]*loginState
}

func newLoginFlows(store *mcpauth.Store, open func(string) bool) *loginFlows {
	return &loginFlows{open: open, store: store, byURL: map[string]*loginState{}}
}

// start begins a login unless one runs for the server; a finished one is replaced.
func (f *loginFlows) start(serverURL string) loginState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st := f.byURL[serverURL]; st != nil && st.State == loginRunning {
		return *st
	}
	st := &loginState{State: loginRunning}
	f.byURL[serverURL] = st
	go func() {
		err := mcpauth.Login(context.Background(), f.store, serverURL, func(authURL string) error {
			opened := f.open(authURL)
			f.mu.Lock()
			st.AuthURL, st.Opened = authURL, opened
			f.mu.Unlock()
			return nil
		})
		f.mu.Lock()
		defer f.mu.Unlock()
		next, msg := loginDone, ""
		if err != nil {
			next, msg = loginFailed, err.Error()
		}
		if st.State.canMoveTo(next) {
			st.State, st.Error = next, msg
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
	mux.HandleFunc("POST /api/mcp/login", func(response http.ResponseWriter, request *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<16)).Decode(&in); err != nil || !httpURL(in.URL) {
			http.Error(response, "want {url: an http(s) url}", http.StatusBadRequest)
			return
		}
		respond(response, f.start(in.URL), nil)
	})
	mux.HandleFunc("GET /api/mcp/login", func(response http.ResponseWriter, request *http.Request) {
		st, ok := f.status(request.URL.Query().Get("url"))
		if !ok {
			http.Error(response, "no login for this url", http.StatusNotFound)
			return
		}
		respond(response, st, nil)
	})
}

func httpURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
}

// remoteLogin is how mcp-proxy logs in: through the background server, which
// opens the browser; in this process only when no server answers.
func remoteLogin(ctx context.Context, serverURL string, shown func(authURL string)) error {
	err := loginViaServer(ctx, serverAddr(), serverURL, shown)
	if !errors.Is(err, errUnavailable) {
		return err
	}
	return mcpauth.Login(ctx, authStore(), serverURL, func(authURL string) error {
		if !tryOpenBrowser(authURL) {
			noBrowser(authURL)
		}
		shown(authURL)
		return nil
	})
}

// noBrowser tells the user where to go when no browser could be started; an
// agent shows the proxy's stderr in its MCP log.
func noBrowser(authURL string) {
	fmt.Fprintln(os.Stderr, "gascoigne: no browser could be opened; open this address to log in:", authURL)
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
			if !st.Opened {
				noBrowser(st.AuthURL)
			}
			shown(st.AuthURL)
			sentURL = true
		}
		switch st.State {
		case loginDone:
			return nil
		case loginFailed:
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
