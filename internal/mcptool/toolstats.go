package mcptool

import "context"

// ToolStatRecorder is implemented by ledgers that keep per-tool MCP stats.
// It is optional and lives here, not in component/, so the public contracts
// stay as they are: receivers type-assert their ledger for it.
type ToolStatRecorder interface {
	// RecordToolList stores the definition sizes in tokens, by tool name, of the
	// tools the agent was shown and, in lazy, of the tools it was not shown
	// (offered on request only), whose definitions it does not pay for.
	RecordToolList(ctx context.Context, agent, server string, tokens, lazy map[string]int) error
	// RecordToolCall counts one call of a tool.
	RecordToolCall(ctx context.Context, agent, server, tool string) error
}
