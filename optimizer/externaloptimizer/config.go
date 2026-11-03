package externaloptimizer

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

// Wire protocols; each has one codec in codecs.
const (
	Text = "text"
	JSON = "json"
	RPC  = "rpc"
)

type Config struct {
	// Command is the argv to run. It is never passed to a shell.
	Command []string `yaml:"command"`
	// Protocol: text (stdin/stdout, metadata in env), json (one request, one
	// response) or rpc (long-lived child, newline-delimited json).
	Protocol string `yaml:"protocol"`
	// Types limits the blocks sent by Block.Type; a trailing /* matches a
	// family (log/*). Empty: every block.
	Types []string `yaml:"types"`
	// Cmds limits by glob on the cmd attribute. Empty: any command. Types and
	// Cmds must both match.
	Cmds []string `yaml:"cmds"`
	// Timeout bounds one call (rpc: one request).
	Timeout time.Duration `yaml:"timeout"`
	// MaxInputTokens skips bigger blocks; 0 sends any size.
	MaxInputTokens int `yaml:"max_input_tokens"`
	// MaxOutputBytes rejects a larger reply.
	MaxOutputBytes int `yaml:"max_output_bytes"`
	// Dir is the working directory; empty keeps the caller's.
	Dir string `yaml:"dir"`
	// Env adds variables to the scrubbed child environment.
	Env map[string]string `yaml:"env"`
	// PassEnv forwards these variables from gascoigne's own environment.
	PassEnv []string `yaml:"pass_env"`
}

func (c *Config) Validate() error {
	if len(c.Command) == 0 || c.Command[0] == "" {
		return errors.New("command is required")
	}
	if _, ok := codecs[c.Protocol]; !ok {
		return fmt.Errorf("protocol must be one of %s", strings.Join(protocolNames(), ", "))
	}
	if c.Timeout <= 0 || c.MaxInputTokens < 0 || c.MaxOutputBytes <= 0 {
		return errors.New("need timeout > 0, max_input_tokens >= 0 and max_output_bytes > 0")
	}
	for _, g := range slices.Concat(c.Types, c.Cmds) {
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("bad pattern %q: %w", g, err)
		}
	}
	return nil
}

// wants reports whether a block of type typ, run with command cmd, goes to the child.
func (c *Config) wants(typ, cmd string) bool {
	return matchAny(c.Types, typ, typeMatch) && matchAny(c.Cmds, cmd, globMatch)
}

func matchAny(patterns []string, s string, match func(pat, s string) bool) bool {
	return len(patterns) == 0 || slices.ContainsFunc(patterns, func(pat string) bool { return match(pat, s) })
}

func typeMatch(pat, typ string) bool {
	if fam, ok := strings.CutSuffix(pat, "/*"); ok {
		return typ == fam || strings.HasPrefix(typ, fam+"/")
	}
	return pat == typ
}

// globMatch matches the whole command line; * also crosses spaces and slashes.
func globMatch(pat, s string) bool {
	parts := strings.Split(pat, "*")
	if len(parts) == 1 {
		return pat == s
	}
	if !strings.HasPrefix(s, parts[0]) || !strings.HasSuffix(s, parts[len(parts)-1]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(s, mid)
		if i < 0 {
			return false
		}
		s = s[i+len(mid):]
	}
	return len(s) >= len(parts[len(parts)-1])
}
