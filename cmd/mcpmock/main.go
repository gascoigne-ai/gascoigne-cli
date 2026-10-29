// Command mcpmock is the fake MCP server of internal/mcpmock.
//
//	mcpmock serve        MCP server on stdio, e.g. behind `gascoigne mcp-proxy --`
//	mcpmock call <tool>  print what an agent gets back from one tool call
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcpmock"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mcpmock:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	switch {
	case len(args) == 1 && args[0] == "serve":
		return mcpmock.NewServer().Run(ctx, &mcp.StdioTransport{})
	case len(args) == 2 && args[0] == "call":
		text, err := mcpmock.Call(ctx, args[1])
		fmt.Print(text)
		return err
	}
	return fmt.Errorf("usage: mcpmock serve | call <tool>")
}
