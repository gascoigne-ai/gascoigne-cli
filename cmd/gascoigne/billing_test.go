package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// billingEnv is a home with none of the sign-ins and no key variables.
func billingEnv(t *testing.T) string {
	t.Helper()
	home := sandbox(t)
	for _, name := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CONFIG_DIR", "OPENAI_API_KEY", "CODEX_HOME", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI",
	} {
		t.Setenv(name, "")
	}
	return home
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectClaudeBilling(t *testing.T) {
	home := billingEnv(t)
	if got := detectBilling("claude"); got.Mode != cost.BillingAPI {
		t.Fatalf("no sign-in at all: %+v", got)
	}
	put(t, filepath.Join(home, ".claude.json"), `{"oauthAccount": {"emailAddress": "a@b.c"}}`)
	if got := detectBilling("claude"); got.Mode != cost.BillingSubscription || got.Why == "" {
		t.Fatalf("signed in with an account: %+v", got)
	}
	os.Remove(filepath.Join(home, ".claude.json"))
	put(t, filepath.Join(home, ".claude", ".credentials.json"), `{"claudeAiOauth": {"subscriptionType": "max"}}`)
	if got := detectBilling("claude"); got.Mode != cost.BillingSubscription {
		t.Fatalf("credentials file: %+v", got)
	}
	// An API key wins: Claude Code uses it instead of the sign-in.
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-x")
	if got := detectBilling("claude"); got.Mode != cost.BillingAPI || got.Why == "" {
		t.Fatalf("key beats sign-in: %+v", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	if got := detectBilling("claude"); got.Mode != cost.BillingAPI {
		t.Fatalf("bedrock is per token: %+v", got)
	}
}

func TestDetectClaudeAPIKeyHelper(t *testing.T) {
	home := billingEnv(t)
	put(t, filepath.Join(home, ".claude.json"), `{"oauthAccount": {}}`)
	put(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "/bin/get-key"}`)
	if got := detectBilling("claude"); got.Mode != cost.BillingAPI {
		t.Fatalf("apiKeyHelper means a key: %+v", got)
	}
}

func TestDetectCodexBilling(t *testing.T) {
	home := billingEnv(t)
	if got := detectBilling("codex"); got.Mode != cost.BillingAPI {
		t.Fatalf("nothing: %+v", got)
	}
	put(t, filepath.Join(home, ".codex", "auth.json"), `{"OPENAI_API_KEY": null, "tokens": {"id_token": "x"}}`)
	if got := detectBilling("codex"); got.Mode != cost.BillingSubscription {
		t.Fatalf("chatgpt sign-in: %+v", got)
	}
	put(t, filepath.Join(home, ".codex", "auth.json"), `{"OPENAI_API_KEY": "sk-x", "tokens": null}`)
	if got := detectBilling("codex"); got.Mode != cost.BillingAPI {
		t.Fatalf("key in auth.json: %+v", got)
	}
	put(t, filepath.Join(home, ".codex", "auth.json"), `{"OPENAI_API_KEY": null, "tokens": {"id_token": "x"}}`)
	t.Setenv("OPENAI_API_KEY", "sk-env")
	if got := detectBilling("codex"); got.Mode != cost.BillingAPI {
		t.Fatalf("env key beats sign-in: %+v", got)
	}
}

func TestDetectGeminiAndOthers(t *testing.T) {
	home := billingEnv(t)
	put(t, filepath.Join(home, ".gemini", "oauth_creds.json"), `{"access_token": "x"}`)
	if got := detectBilling("gemini"); got.Mode != cost.BillingSubscription {
		t.Fatalf("google sign-in: %+v", got)
	}
	t.Setenv("GEMINI_API_KEY", "k")
	if got := detectBilling("gemini"); got.Mode != cost.BillingAPI {
		t.Fatalf("key: %+v", got)
	}
	if got := detectBilling("cursor"); got.Mode != cost.BillingSubscription {
		t.Fatalf("cursor bills a plan: %+v", got)
	}
	for _, name := range []string{"opencode", "pi", "langgraph", ""} {
		if got := detectBilling(name); got.Mode != cost.BillingAPI {
			t.Fatalf("%q unknown agents default to the API: %+v", name, got)
		}
	}
}

func TestDetectedAgentsListsTheKnownOnes(t *testing.T) {
	billingEnv(t)
	got := detectedAgents()
	names := map[string]bool{}
	for _, d := range got {
		names[d.Agent] = true
		if d.Mode == "" || d.Why == "" {
			t.Errorf("incomplete: %+v", d)
		}
	}
	for _, want := range []string{"claude", "codex", "gemini", "cursor"} {
		if !names[want] {
			t.Errorf("%s missing from %v", want, got)
		}
	}
}
