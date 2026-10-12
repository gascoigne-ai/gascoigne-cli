package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gascoigne-ai/gascoigne-cli/mcpauth"
)

// authStore is where mcp-login keeps the OAuth tokens of remote MCP servers.
func authStore() *mcpauth.Store {
	return &mcpauth.Store{Path: filepath.Join(stateDir(), "mcp-auth.json")}
}

func cmdMCPLogin(ctx context.Context, args []string) error {
	fs := newFlags("mcp-login")
	logout := fs.Bool("logout", false, "forget the saved login of the server")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: gascoigne mcp-login [-logout] <url>")
	}
	url, store := fs.Arg(0), authStore()
	if *logout {
		found, err := store.Delete(url)
		switch {
		case err != nil:
			return err
		case !found:
			fmt.Println("no saved login for", mcpauth.Display(url))
		default:
			fmt.Println("forgot the login for", mcpauth.Display(url))
		}
		return nil
	}
	open := func(authURL string) error {
		fmt.Println("Authorize gascoigne in your browser (opening it); if nothing opens, visit:\n ", authURL)
		openBrowser(authURL)
		return nil
	}
	if err := mcpauth.Login(ctx, store, url, open); err != nil {
		return err
	}
	fmt.Println("logged in to", mcpauth.Display(url))
	return nil
}
