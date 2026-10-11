package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/cost"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/tokenizer"
	"github.com/lukaszb/gascoigne/tokens"
)

// cmdTokens counts text the ways models do and prices one read of it.
func cmdTokens(ctx context.Context, args []string) error {
	fs := newFlags("tokens")
	cfgFile := fs.String("config", "", "config `file` (default: $GASCOIGNE_CONFIG, $XDG_CONFIG_HOME/gascoigne/config.yaml, built-in)")
	exact := fs.Bool("exact", true, "also count exactly with the configured tokenizers (o200k, Claude via API)")
	models := fs.String("model", "claude-opus-5-5,claude-sonnet-5-5,gpt-5.5,gemini-3.1-pro", "models to price, a comma `list`")
	rereads := fs.Float64("rereads", cost.AgentContext.Rereads, "later requests that read it again while in an agent's context (`n`)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var in io.Reader = os.Stdin
	if fs.NArg() > 0 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	text := string(data)
	est := tokens.Estimate(text)
	w := os.Stdout
	fmt.Fprintf(w, "%s bytes · %d lines\n\n", human(len(text)), strings.Count(text, "\n"))
	fmt.Fprintf(w, "%-14s %9s  %s\n", "tokenizer", "tokens", "basis")
	fmt.Fprintf(w, "%-14s %9d  %s\n", "bytes/4", (len(text)+3)/4, "rule of thumb; overcounts whitespace, undercounts Claude")
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for n := range cost.Tokenizers {
			if !yield(n) {
				return
			}
		}
	}) {
		t := cost.Tokenizers[name]
		fmt.Fprintf(w, "%-14s %9d  %s\n", name, int(float64(est)*t.Factor+0.5), t.Basis)
	}
	if *exact {
		cfg, err := (&common{config: *cfgFile}).load()
		if err != nil {
			return err
		}
		exactCounts(ctx, w, cfg.Tokenizers, text)
	}
	fmt.Fprintf(w, "\n%-20s %10s %12s %12s\n", "model", "tokens", "one read", "in context")
	c := cost.Context{Rereads: *rereads, Miss: cost.AgentContext.Miss}
	for _, id := range splitList(*models, ",") {
		m, ok := cost.Lookup(id)
		if !ok {
			fmt.Fprintf(w, "%-20s %s\n", id, "unknown model")
			continue
		}
		s := cost.Estimate(m, est, c)
		fmt.Fprintf(w, "%-20s %10d %12s %12s\n", cmp.Or(m.ID, id), s.ModelTokens, cents(s.FirstRead), cents(s.InContext))
	}
	fmt.Fprintf(w, "\none read: cache write (or plain input); in context: plus %.0f cache reads, as an agent rereads a tool result\n", *rereads)
	return nil
}

// cents prints small amounts readably: $0.0123.
func cents(v float64) string {
	if v < 1 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// exactCounts prints counts of the configured exact tokenizers; one that
// cannot count says why instead.
func exactCounts(ctx context.Context, w io.Writer, t config.Tokenizers, text string) {
	if t.O200k {
		if dir, err := os.UserCacheDir(); err != nil {
			fmt.Fprintf(w, "%-14s %9s  %s\n", "o200k exact", "-", err)
		} else if bpe, err := tokenizer.O200k(ctx, filepath.Join(dir, "gascoigne")); err != nil {
			fmt.Fprintf(w, "%-14s %9s  %s\n", "o200k exact", "-", err)
		} else {
			fmt.Fprintf(w, "%-14s %9d  %s\n", "o200k exact", bpe.Count(text), "o200k_base, counted locally")
		}
	}
	key := cmp.Or(t.AnthropicAPIKey, os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		fmt.Fprintf(w, "%-14s %9s  %s\n", "claude exact", "-", "set tokenizers.anthropic_api_key in the config or ANTHROPIC_API_KEY")
		return
	}
	n, err := tokenizer.Claude{Key: key, Model: t.AnthropicModel}.Count(ctx, text)
	if err != nil {
		fmt.Fprintf(w, "%-14s %9s  %s\n", "claude exact", "-", err)
		return
	}
	fmt.Fprintf(w, "%-14s %9d  %s\n", "claude exact", n, "Anthropic count_tokens, "+t.AnthropicModel)
}
