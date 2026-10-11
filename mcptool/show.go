package mcptool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/compress/symbols"
	"github.com/lukaszb/gascoigne/detect"
)

// Show reads one definition of a source file with its line numbers, or with
// symbol "" the file's outline: each definition's signature line, numbered,
// so an agent reads only the function it needs.
func Show(path, symbol string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(b)
	defs, err := symbols.Defs(text, detect.Hints{File: path})
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var out strings.Builder
	if symbol == "" {
		fmt.Fprintf(&out, "%s: %d lines, %d definitions\n", path, len(lines), len(defs))
		for _, d := range defs {
			fmt.Fprintf(&out, "%d-%d\t%s\n", d.Line, d.To, strings.TrimSpace(lines[d.Line-1]))
		}
		return out.String(), nil
	}
	d, err := symbols.Find(defs, symbol)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	fmt.Fprintf(&out, "%s:%d-%d %s %s\n", path, d.From, d.To, d.Kind, d.Name)
	for i := d.From; i <= d.To; i++ {
		fmt.Fprintf(&out, "%d\t%s\n", i, lines[i-1])
	}
	return out.String(), nil
}

type ShowInput struct {
	File   string `json:"file" jsonschema:"source file path"`
	Symbol string `json:"symbol,omitempty" jsonschema:"function, method (Type.Method) or type; empty lists the file's definitions"`
}

// AddShow registers a tool reading one definition or a file's outline.
func AddShow(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "show",
		Description: "Read one function, method or type of a source file with line numbers (Go, Python, JS/TS, Java, C#, Rust, C…); without symbol, list its definitions.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ShowInput) (*mcp.CallToolResult, any, error) {
		text, err := Show(in.File, in.Symbol)
		if err != nil {
			return nil, nil, err
		}
		return Result(text), nil, nil
	})
}
