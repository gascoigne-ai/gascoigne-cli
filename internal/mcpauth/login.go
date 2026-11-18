package mcpauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

// LoginTimeout is how long Login waits for the user in the browser.
const LoginTimeout = 5 * time.Minute

// Login runs the authorization code flow (PKCE, dynamic client registration)
// against a server: open shows the authorization page to the user (a browser,
// or a printed address), and the tokens are saved in the store.
func Login(ctx context.Context, store *Store, serverURL string, open func(authURL string) error) error {
	ctx, cancel := context.WithTimeout(ctx, LoginTimeout)
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for the redirect: %w", err)
	}
	defer ln.Close()
	redirect := "http://" + ln.Addr().String() + "/callback"
	results := serveCallback(ln)

	h, err := loginHandler(store, serverURL, redirect, results, open)
	if err != nil {
		return err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "gascoigne-login", Version: version.Number}, nil)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: serverURL, OAuthHandler: h}, nil)
	if err != nil {
		return fmt.Errorf("log in to %s: %w", Display(serverURL), err)
	}
	cs.Close()
	if _, ok := store.Get(serverURL); !ok {
		return errors.New("the server did not ask for a login (OAuth)")
	}
	return nil
}

// discoveryClient returns the client for OAuth metadata requests. The user
// chose this server, so the SDK's private-address guard (lifted by any transport
// that is not a plain *http.Transport) only gets in the way of internal servers.
func discoveryClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	return &http.Client{Transport: trimWellKnown{t}}
}

// trimWellKnown drops the trailing slash the SDK leaves on metadata paths when
// the issuer is a bare origin with a slash ("https://host/"): proxies answer
// ".../oauth-authorization-server/" with a redirect to http and a 404.
type trimWellKnown struct{ http.RoundTripper }

func (t trimWellKnown) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasPrefix(request.URL.Path, "/.well-known/") && strings.HasSuffix(request.URL.Path, "/") {
		request = request.Clone(request.Context())
		request.URL.Path = strings.TrimSuffix(request.URL.Path, "/")
	}
	return t.RoundTripper.RoundTrip(request)
}

// serveCallback answers the browser's redirect once and hands the result on.
func serveCallback(ln net.Listener) <-chan *auth.AuthorizationResult {
	results := make(chan *auth.AuthorizationResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(writer http.ResponseWriter, request *http.Request) {
		q := request.URL.Query()
		if q.Get("code") == "" {
			http.Error(writer, "login failed: "+q.Get("error"), http.StatusBadRequest)
			return
		}
		select {
		case results <- &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}:
		default:
		}
		fmt.Fprintln(writer, "Logged in. You can close this tab and return to the terminal.")
	})
	go (&http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
	return results
}

// loginHandler is the OAuth handler of a login: it registers a client for the redirect address,
// hands the authorization address to open, waits for the browser's answer on results and saves
// the tokens in the store.
func loginHandler(store *Store, serverURL, redirect string, results <-chan *auth.AuthorizationResult, open func(authURL string) error) (*auth.AuthorizationCodeHandler, error) {
	return auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			RedirectURIs:            []string{redirect},
			ClientName:              "gascoigne",
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none",
		}},
		Client:              discoveryClient(),
		RedirectURL:         redirect,
		RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			if err := open(args.URL); err != nil {
				return nil, err
			}
			select {
			case r := <-results:
				return r, nil
			case <-ctx.Done():
				return nil, fmt.Errorf("no answer from the browser: %w", ctx.Err())
			}
		},
		NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
			c := Creds{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: cfg.Endpoint.TokenURL, AuthStyle: cfg.Endpoint.AuthStyle, Token: tok}
			if err := store.Put(serverURL, c); err != nil {
				return nil, err
			}
			return saving(ctx, store, serverURL, c), nil
		},
	})
}
