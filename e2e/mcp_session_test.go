package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Call is one MCP tool call made by a step's client. Args values starting with "@" are
// replaced by a repo fixture's content, so a tool can be fed real tool output.
type Call struct {
	Tool    string
	Args    map[string]any
	Text    []Check           // over the result's text
	Save    map[string]string // variable name -> regexp with one group, matched over the text
	IsError bool              // the tool reports an error result
}

// runMCP starts the step's command as an MCP server on stdio, lists its tools and makes
// each call in order. Stdout of the result is the tool names, one per line, so a step's
// Stdout checks judge the tool list; Call.Text judges each result.
func (box *sandbox) runMCP(t *testing.T, step Step, words []string) result {
	ctx, cancel := context.WithTimeout(t.Context(), stepTimeout)
	defer cancel()
	cmd := exec.Command(filepath.Join(box.vars["bin"], words[0]), words[1:]...)
	cmd.Dir = box.vars["work"]
	cmd.Env = append(append([]string{}, box.env...), box.expandAll(step.Env)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return result{stderr: stderr.String(), failure: fmt.Errorf("connect to %q: %w", step.Run, err)}
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return result{stderr: stderr.String(), failure: fmt.Errorf("list tools: %w", err)}
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	got := result{stdout: strings.Join(names, "\n") + "\n"}
	for number, call := range step.MCP {
		if err := box.callTool(ctx, session, call); err != nil {
			got.failure = fmt.Errorf("call %d (%s): %w", number+1, call.Tool, err)
			break
		}
	}
	got.stderr = stderr.String()
	return got
}

func (box *sandbox) callTool(ctx context.Context, session *mcp.ClientSession, call Call) error {
	args, err := box.callArgs(call.Args)
	if err != nil {
		return err
	}
	reply, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.Tool, Arguments: args})
	if err != nil {
		return err
	}
	var text strings.Builder
	for _, content := range reply.Content {
		if block, ok := content.(*mcp.TextContent); ok {
			text.WriteString(block.Text)
		}
	}
	if reply.IsError != call.IsError {
		return fmt.Errorf("IsError %v, want %v; text: %s", reply.IsError, call.IsError, text.String())
	}
	for _, check := range call.Text {
		if err := check(box, text.String()); err != nil {
			return fmt.Errorf("text: %w", err)
		}
	}
	return box.save(call.Save, text.String())
}

func (box *sandbox) callArgs(args map[string]any) (map[string]any, error) {
	expanded := map[string]any{}
	for name, value := range args {
		text, ok := value.(string)
		if !ok {
			expanded[name] = value
			continue
		}
		text = box.expand(text)
		if strings.HasPrefix(text, "@") {
			data, err := loadContent(text)
			if err != nil {
				return nil, err
			}
			text = string(data)
		}
		expanded[name] = text
	}
	return expanded, nil
}
