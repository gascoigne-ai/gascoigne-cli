package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// How an agent is billed decides whether dollar figures mean anything: a subscription bills a
// flat fee, the API bills per token. Nothing says so in a ledger run, but each agent leaves a
// trace of how it is signed in on this machine, which detectBilling reads. It only looks at
// whether a key variable is set and which sign-in file exists: it never reads a secret's value.

// billingDetectors are the agents gascoigne can tell apart. Any other agent is taken to be
// on the API.
var billingDetectors = map[string]func() cost.Detection{
	"claude": detectClaude,
	"codex":  detectCodex,
	"gemini": detectGemini,
	"cursor": func() cost.Detection {
		return cost.Detection{Mode: cost.BillingSubscription, Why: "Cursor bills a plan"}
	},
}

// detectBilling is cost's detector.
func detectBilling(agent string) cost.Detection {
	if detect, ok := billingDetectors[agent]; ok {
		return detect()
	}
	return cost.Detection{Mode: cost.BillingAPI, Why: "no way to tell for this agent"}
}

// detected is one agent's billing, for display.
type detected struct {
	Agent string `json:"agent"`
	Mode  string `json:"mode"`
	Why   string `json:"why"`
}

// detectedAgents lists what auto finds for every agent it can tell apart.
func detectedAgents() []detected {
	var out []detected
	for _, name := range slices.Sorted(mapKeys(billingDetectors)) {
		found := billingDetectors[name]()
		out = append(out, detected{Agent: name, Mode: found.Mode, Why: found.Why})
	}
	return out
}

func mapKeys[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for key := range m {
			if !yield(key) {
				return
			}
		}
	}
}

func api(why string) cost.Detection { return cost.Detection{Mode: cost.BillingAPI, Why: why} }

func plan(why string) cost.Detection { return cost.Detection{Mode: cost.BillingSubscription, Why: why} }

func envSet(names ...string) string {
	for _, name := range names {
		if os.Getenv(name) != "" {
			return name
		}
	}
	return ""
}

// readJSON decodes a small file into a generic map; a missing or broken one is nil.
func readJSON(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

func home() string {
	dir, _ := os.UserHomeDir()
	return dir
}

// detectClaude: a key, a Bedrock/Vertex/Foundry setup or an apiKeyHelper means per-token billing;
// a Claude account sign-in means a plan.
func detectClaude() cost.Detection {
	if name := envSet("CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"); name != "" {
		return api(name + " is set: billed per token by the cloud provider")
	}
	if name := envSet("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"); name != "" {
		return api(name + " is set")
	}
	config := os.Getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		config = filepath.Join(home(), ".claude")
	}
	if settings := readJSON(filepath.Join(config, "settings.json")); settings["apiKeyHelper"] != nil {
		return api("settings.json names an apiKeyHelper")
	}
	if creds := readJSON(filepath.Join(config, ".credentials.json")); creds["claudeAiOauth"] != nil {
		return plan("signed in with a Claude account")
	}
	// Where the credentials sit in a keychain (macOS), the account is still named here.
	if state := readJSON(filepath.Join(home(), ".claude.json")); state["oauthAccount"] != nil {
		return plan("signed in with a Claude account")
	}
	return api("no Claude account sign-in found")
}

// detectCodex: a key in the environment or in auth.json means per-token billing; a ChatGPT
// sign-in (tokens, no key) means a plan.
func detectCodex() cost.Detection {
	if name := envSet("OPENAI_API_KEY"); name != "" {
		return api(name + " is set")
	}
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		dir = filepath.Join(home(), ".codex")
	}
	auth := readJSON(filepath.Join(dir, "auth.json"))
	if key, _ := auth["OPENAI_API_KEY"].(string); key != "" {
		return api("auth.json holds an API key")
	}
	if auth["tokens"] != nil {
		return plan("signed in with a ChatGPT account")
	}
	return api("no ChatGPT sign-in found")
}

// detectGemini: a key or Vertex means per-token billing; a Google sign-in means the free tier or
// a Code Assist plan.
func detectGemini() cost.Detection {
	if name := envSet("GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI"); name != "" {
		return api(name + " is set")
	}
	if _, err := os.Stat(filepath.Join(home(), ".gemini", "oauth_creds.json")); err == nil {
		return plan("signed in with a Google account (free tier or Code Assist)")
	}
	return api("no Google account sign-in found")
}
