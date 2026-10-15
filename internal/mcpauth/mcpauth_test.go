package mcpauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcpauth/authtest"
)

func TestLoginThenHandler(t *testing.T) {
	url, refreshed := authtest.Server(t)
	store := &Store{Path: filepath.Join(t.TempDir(), "state", "mcp-auth.json")}

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

func TestDiscoveryClientOptsOutOfPrivateIPGuard(t *testing.T) {
	// The SDK keeps its private-address check only for a plain *http.Transport without a custom dialer.
	if tr, ok := discoveryClient().Transport.(trimWellKnown).RoundTripper.(*http.Transport); !ok || tr.DialContext == nil {
		t.Fatal("no dialer set: private-address servers cannot log in")
	}
}

func TestDiscoveryClientTrimsWellKnownSlash(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { paths = append(paths, r.URL.Path) }))
	defer srv.Close()

	c := discoveryClient()
	for _, p := range []string{"/.well-known/oauth-authorization-server/", "/.well-known/oauth-protected-resource/mcp", "/register/"} {
		resp, err := c.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	want := []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp", "/register/"}
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %q, want %q", paths, want)
	}
}

func TestHandlerAuthorizeNamesTheFix(t *testing.T) {
	h := &Handler{url: "https://u:p@mcp.example.com/mcp?key=secret"}
	err := h.Authorize(t.Context(), nil, &http.Response{Body: io.NopCloser(strings.NewReader(""))})
	if err == nil || !strings.Contains(err.Error(), "gascoigne mcp-login https://mcp.example.com/mcp") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
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
