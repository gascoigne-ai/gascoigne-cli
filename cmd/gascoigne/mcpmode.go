package main

import (
	"os"
	"slices"
)

// MCP modes: how gascoigne reaches the agent.
//
//   - auto (default): hooks route tool results through gascoigne (Bash rewritten to
//     gascoigne exec, results replaced after the call, MCP results included) and
//     claude's Grep, Glob and WebFetch are denied so searches and pages come through it.
//   - manual: nothing is routed or denied. The agent calls gascoigne's own MCP tools
//     (fetch, compress, retrieve, show, stats) itself when it wants a result shortened.
//     Session start still adds the guide, saying so.
const (
	mcpAuto   = "auto"
	mcpManual = "manual"
)

var mcpModes = []string{mcpAuto, mcpManual}

// mcpMode is $GASCOIGNE_MCP_MODE, else the saved pref, else auto. An unknown value
// is ignored: a typo must not silently turn routing off.
func mcpMode() string {
	for _, v := range []string{os.Getenv("GASCOIGNE_MCP_MODE"), readPrefs().MCPMode} {
		if slices.Contains(mcpModes, v) {
			return v
		}
	}
	return mcpAuto
}

func saveMCPMode(v string) error {
	return savePref("mcp", v, mcpModes, func(p *prefs, v string) { p.MCPMode = v })
}

// routesTools reports whether a hook event is one that routes a tool call or its
// result. Session start (the guide) and prompt events (the server, nudges) are not.
func routesTools(event string) bool {
	return event != "SessionStart" && event != "UserPromptSubmit"
}

// skipHook is true when manual mode leaves this hook event alone.
func skipHook(event string) bool {
	return mcpMode() == mcpManual && routesTools(event)
}

// deniedTools are the claude tools install denies: none in manual mode.
func deniedTools() []string {
	if mcpMode() == mcpManual {
		return nil
	}
	return claudeDenies
}
