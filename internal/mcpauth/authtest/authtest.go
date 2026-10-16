// Package authtest is a fake MCP server behind OAuth, for tests: protected-resource
// and authorization-server metadata, dynamic registration, authorize, token.
package authtest

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is an MCP server behind OAuth: protected-resource and
// authorization-server metadata, dynamic registration, authorize, token.
func Server(t *testing.T) (base string, refreshed *int) {
	t.Helper()
	n := 0
	mux := http.NewServeMux()
	var srv *httptest.Server
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcp.NewServer(&mcp.Implementation{Name: "secured", Version: "1"}, nil)
	}, nil)
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer tok1" && auth != "Bearer tok2" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource"`)
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"resource": srv.URL + "/mcp", "authorization_servers": []string{srv.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize", "token_endpoint": srv.URL + "/token",
			"registration_endpoint": srv.URL + "/register", "code_challenge_methods_supported": []string{"S256"},
			"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		m["client_id"] = "cid"
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, m)
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Clients under test listen on a loopback port; refuse to redirect anywhere else.
		cb, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || cb.Scheme != "http" || !isLoopback(cb.Hostname()) {
			http.Error(w, "redirect_uri must be a loopback http URL", http.StatusBadRequest)
			return
		}
		cb.RawQuery = url.Values{"code": {"c1"}, "state": {q.Get("state")}}.Encode()
		http.Redirect(w, r, cb.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		tok, expires := "tok1", 3600
		if r.PostForm.Get("grant_type") == "refresh_token" {
			n++
			tok = "tok2"
		} else {
			expires = 1 // inside the refresh margin: the first use refreshes
		}
		writeJSON(w, map[string]any{"access_token": tok, "token_type": "Bearer", "refresh_token": "r1", "expires_in": expires})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp", &n
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
