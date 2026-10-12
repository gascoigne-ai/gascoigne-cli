package mcptool

import "context"

// ToolStatRecorder is implemented by ledgers that keep per-tool MCP stats.
// It is optional and lives here, not in component/, so the public contracts
// stay as they are: receivers type-assert their ledger for it.
type ToolStatRecorder interface {
	// RecordToolList stores each tool's definition size in tokens, by tool name.
	RecordToolList(ctx context.Context, agent, server string, tokens map[string]int) error
	// RecordToolCall counts one call of a tool.
	RecordToolCall(ctx context.Context, agent, server, tool string) error
}
