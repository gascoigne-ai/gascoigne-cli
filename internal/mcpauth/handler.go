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

// NewHandler is NewHandlerContext without a caller context.
func NewHandler(store *Store, serverURL string) *Handler {
	return NewHandlerContext(context.Background(), store, serverURL)
}

// NewHandlerContext returns the handler for a server with a stored login, nil
// without one. Token refreshes run under ctx.
func NewHandlerContext(ctx context.Context, store *Store, serverURL string) *Handler {
	if store == nil {
		return nil
	}
	c, ok := store.Get(serverURL)
	if !ok {
		return nil
	}
	return &Handler{url: serverURL, src: saving(ctx, store, serverURL, c)}
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
func saving(ctx context.Context, store *Store, serverURL string, creds Creds) oauth2.TokenSource {
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: creds.TokenURL, AuthStyle: creds.AuthStyle},
	}
	return &saver{src: cfg.TokenSource(ctx, creds.Token), store: store, url: serverURL, creds: creds}
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
