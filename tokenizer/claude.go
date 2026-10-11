package tokenizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrNoKey means no Anthropic API key is configured.
var ErrNoKey = errors.New("no Anthropic API key: set tokenizers.anthropic_api_key in the config or ANTHROPIC_API_KEY")

// Claude counts tokens as a Claude model reads them, with Anthropic's
// token counting endpoint (free, rate limited).
type Claude struct {
	Key   string
	Model string
	URL   string // API base, https://api.anthropic.com by default
}

// Count is the tokens text adds to a request: the count of a user message
// holding it, minus the count of one holding a single character (the
// message frame), plus one for that character.
func (c Claude) Count(ctx context.Context, text string) (int, error) {
	if c.Key == "" {
		return 0, ErrNoKey
	}
	n, err := c.request(ctx, text)
	if err != nil {
		return 0, err
	}
	frame, err := c.request(ctx, ".")
	if err != nil {
		return 0, err
	}
	return n - frame + 1, nil
}

func (c Claude) request(ctx context.Context, text string) (int, error) {
	body, err := json.Marshal(map[string]any{
		"model":    c.Model,
		"messages": []any{map[string]any{"role": "user", "content": text}},
	})
	if err != nil {
		return 0, err
	}
	base := c.URL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages/count_tokens", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", c.Key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	// Straight to the API: gascoigne's own proxy would only pass it through.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("count_tokens: %s: %s", resp.Status, bytes.TrimSpace(data))
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, fmt.Errorf("count_tokens: %w", err)
	}
	return out.InputTokens, nil
}
