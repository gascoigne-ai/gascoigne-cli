package mcpauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// oauthServer is an MCP server behind OAuth: protected-resource and
// authorization-server metadata, dynamic registration, authorize, token.
func oauthServer(t *testing.T) (base string, refreshed *int) {
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
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=c1&state="+q.Get("state"), http.StatusFound)
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

func TestLoginThenHandler(t *testing.T) {
	url, refreshed := oauthServer(t)
	store := &Store{Path: filepath.Join(t.TempDir(), "state", "mcp-auth.json")}

	if !NeedsOAuth(t.Context(), url) {
		t.Fatal("a 401 with a Bearer challenge is not detected")
	}
	if NewHandler(store, url) != nil {
		t.Fatal("handler without a login")
	}
	open := func(authURL string) error {
		resp, err := http.Get(authURL) // the "browser": follows the redirect to the callback
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if body, _ := io.ReadAll(resp.Body); resp.StatusCode != 200 {
			t.Errorf("callback answered %d: %s", resp.StatusCode, body)
		}
		return nil
	}
	if err := Login(t.Context(), store, url, open); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(url); !ok {
		t.Fatal("login not saved")
	}

	h := NewHandler(store, url)
	if h == nil {
		t.Fatal("no handler after login")
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(t.Context(),
		&mcp.StreamableClientTransport{Endpoint: url, OAuthHandler: h}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs.Close()
	if *refreshed == 0 {
		t.Error("the expired token was not refreshed")
	}
	if c, _ := store.Get(url); c.Token.AccessToken != "tok2" {
		t.Errorf("refreshed token not saved: %+v", c.Token)
	}
}

func TestHandlerAuthorizeNamesTheFix(t *testing.T) {
	h := &Handler{url: "https://u:p@mcp.example.com/mcp?key=secret"}
	err := h.Authorize(t.Context(), nil, &http.Response{Body: io.NopCloser(strings.NewReader(""))})
	if err == nil || !strings.Contains(err.Error(), "gascoigne mcp-login https://mcp.example.com/mcp") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestNeedsOAuthIgnoresOtherAnswers(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"open": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) },
		"basic auth": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.WriteHeader(401)
		},
		"no challenge": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			if NeedsOAuth(context.Background(), srv.URL) {
				t.Fatal("detected OAuth")
			}
		})
	}
	t.Run("probe off", func(t *testing.T) {
		t.Setenv("GASCOIGNE_OAUTH_PROBE", "off")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(401)
		}))
		defer srv.Close()
		if NeedsOAuth(t.Context(), srv.URL) {
			t.Fatal("probe ran while off")
		}
	})
}

func TestStoreDelete(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "a.json")}
	_ = s.Put("https://x/mcp", Creds{ClientID: "c", Token: nil})
	if found, err := s.Delete("https://x/mcp"); !found || err != nil {
		t.Fatalf("delete = %v, %v", found, err)
	}
	if found, _ := s.Delete("https://x/mcp"); found {
		t.Fatal("deleted twice")
	}
}
