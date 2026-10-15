package mcpauth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// Handler is the proxy's side of OAuth: it sends the stored token, refreshing
// it (and saving the new one) as needed, and never starts a login of its own.
type Handler struct {
	url string
	src oauth2.TokenSource
}

// NewHandler returns the handler for a server with a stored login, nil without one.
func NewHandler(s *Store, serverURL string) *Handler {
	if s == nil {
		return nil
	}
	c, ok := s.Get(serverURL)
	if !ok {
		return nil
	}
	return &Handler{url: serverURL, src: saving(s, serverURL, c)}
}

func (h *Handler) TokenSource(context.Context) (oauth2.TokenSource, error) { return h.src, nil }

// Authorize runs when the server refused the token: the refresh token is gone
// or revoked, which only a new login fixes.
func (h *Handler) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	resp.Body.Close()
	return fmt.Errorf("%w: the server refused the saved login; run: gascoigne mcp-login %s", ErrLoginNeeded, Display(h.url))
}

// saving wraps the refreshing token source of a login so a renewed token is
// written back: refresh tokens are often single-use.
func saving(s *Store, serverURL string, c Creds) oauth2.TokenSource {
	cfg := &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: c.TokenURL, AuthStyle: c.AuthStyle},
	}
	return &saver{src: cfg.TokenSource(context.Background(), c.Token), store: s, url: serverURL, creds: c}
}

type saver struct {
	src   oauth2.TokenSource
	store *Store
	url   string
	creds Creds
}

func (v *saver) Token() (*oauth2.Token, error) {
	t, err := v.src.Token()
	if err != nil {
		return nil, err
	}
	if v.creds.Token == nil || t.AccessToken != v.creds.Token.AccessToken {
		v.creds.Token = t
		// Best effort: the token works for this run; a failed save costs a refresh next time.
		_ = v.store.Put(v.url, v.creds)
	}
	return t, nil
}
