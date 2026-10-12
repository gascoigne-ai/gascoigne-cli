// Package mcpauth is OAuth for remote MCP servers behind mcp-proxy: probe
// whether a server asks for it, log in once in a browser (gascoigne mcp-login)
// and keep the tokens where the proxy finds them, refreshed as they expire.
// The proxy itself never opens a browser: an agent starts it without a user at hand.
package mcpauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// ErrLoginNeeded means the stored login is missing, expired for good or refused.
var ErrLoginNeeded = errors.New("login needed")

// Creds is one server's login: the client the server registered for us and its tokens.
type Creds struct {
	ClientID     string           `json:"client_id"`
	ClientSecret string           `json:"client_secret,omitempty"`
	TokenURL     string           `json:"token_url"`
	AuthStyle    oauth2.AuthStyle `json:"auth_style,omitempty"`
	Token        *oauth2.Token    `json:"token"`
}

// Store keeps logins in one file (0600), by a hash of the server url without
// query or credentials: urls can hold keys, and the login is the server's, not the key's.
type Store struct {
	Path string
	mu   sync.Mutex
}

func key(serverURL string) string {
	sum := sha256.Sum256([]byte(Display(serverURL)))
	return hex.EncodeToString(sum[:8])
}

// Display is the url without credentials or query, safe for messages and logs.
func Display(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "the remote server"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}

func (s *Store) read() map[string]Creds {
	all := map[string]Creds{}
	if data, err := os.ReadFile(s.Path); err == nil {
		_ = json.Unmarshal(data, &all) // an unreadable file reads as no logins; the next login rewrites it
	}
	return all
}

// Get returns the login of a server.
func (s *Store) Get(serverURL string) (Creds, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.read()[key(serverURL)]
	return c, ok && c.Token != nil
}

// Put saves a server's login.
func (s *Store) Put(serverURL string, c Creds) error {
	return s.update(func(all map[string]Creds) { all[key(serverURL)] = c })
}

// Delete removes a server's login; false when there was none.
func (s *Store) Delete(serverURL string) (bool, error) {
	found := false
	err := s.update(func(all map[string]Creds) {
		_, found = all[key(serverURL)]
		delete(all, key(serverURL))
	})
	return found, err
}

func (s *Store) update(edit func(map[string]Creds)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.read()
	edit(all)
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("save login: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("save login: %w", err)
	}
	return os.Rename(tmp, s.Path)
}

// NeedsOAuth asks a server for its tools without credentials, quickly: true
// when it answers 401 with a Bearer challenge. Unreachable servers and any
// other answer are false, so a flaky network never turns a server away.
// GASCOIGNE_OAUTH_PROBE=off skips the request.
func NeedsOAuth(ctx context.Context, serverURL string) bool {
	if os.Getenv("GASCOIGNE_OAUTH_PROBE") == "off" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"gascoigne-probe","version":"0"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusUnauthorized && strings.Contains(strings.ToLower(strings.Join(resp.Header.Values("WWW-Authenticate"), " ")), "bearer")
}
