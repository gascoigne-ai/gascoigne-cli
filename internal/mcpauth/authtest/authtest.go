// Package authtest is a fake MCP server behind OAuth, for tests: protected-resource
// and authorization-server metadata, dynamic registration, authorize, token.
package authtest

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	mux.HandleFunc("/mcp", func(writer http.ResponseWriter, request *http.Request) {
		if auth := request.Header.Get("Authorization"); auth != "Bearer tok1" && auth != "Bearer tok2" {
			writer.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource"`)
			http.Error(writer, "no", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(writer, request)
	})
	writeJSON := func(writer http.ResponseWriter, value any) {
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(value)
	}
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, map[string]any{"resource": srv.URL + "/mcp", "authorization_servers": []string{srv.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, map[string]any{
			"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize", "token_endpoint": srv.URL + "/token",
			"registration_endpoint": srv.URL + "/register", "code_challenge_methods_supported": []string{"S256"},
			"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	})
	mux.HandleFunc("/register", func(writer http.ResponseWriter, request *http.Request) {
		var m map[string]any
		json.NewDecoder(request.Body).Decode(&m)
		m["client_id"] = "cid"
		writer.WriteHeader(http.StatusCreated)
		writeJSON(writer, m)
	})
	mux.HandleFunc("/authorize", func(writer http.ResponseWriter, request *http.Request) {
		q := request.URL.Query()
		// Clients under test listen on a loopback port; refuse to redirect anywhere else.
		// The host is one of three literals and the port a number, so the target is
		// never taken from the request.
		cb, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			http.Error(writer, "redirect_uri is not a URL", http.StatusBadRequest)
			return
		}
		host, ok := loopbackHosts[cb.Hostname()]
		port, portErr := strconv.Atoi(cb.Port())
		if cb.Scheme != "http" || !ok || portErr != nil || cb.Path != callbackPath {
			http.Error(writer, "redirect_uri must be http://<loopback>:<port>"+callbackPath, http.StatusBadRequest)
			return
		}
		target := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + callbackPath
		http.Redirect(writer, request, target+"?"+url.Values{"code": {"c1"}, "state": {q.Get("state")}}.Encode(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(writer http.ResponseWriter, request *http.Request) {
		request.ParseForm()
		tok, expires := "tok1", 3600
		if request.PostForm.Get("grant_type") == "refresh_token" {
			n++
			tok = "tok2"
		} else {
			expires = 1 // inside the refresh margin: the first use refreshes
		}
		writeJSON(writer, map[string]any{"access_token": tok, "token_type": "Bearer", "refresh_token": "r1", "expires_in": expires})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp", &n
}

// callbackPath is where mcpauth.Login listens.
const callbackPath = "/callback"

// loopbackHosts maps a callback host to the literal the redirect is built from.
var loopbackHosts = map[string]string{"127.0.0.1": "127.0.0.1", "localhost": "localhost", "::1": "::1"}
