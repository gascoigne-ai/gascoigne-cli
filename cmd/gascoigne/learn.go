package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// Lessons (experimental: learn). The ledger shows habits that cost context:
// running a command again for output gascoigne already shortened (the rest is
// one `gascoigne raw` away), and commands whose large output nothing can
// shorten. A habit seen in several sessions becomes one line for a local
// instruction file. Nothing is written without -apply, and -remove takes the
// block out again.

const (
	learnMinSessions = 2    // a habit must show in this many sessions
	learnMinRuns     = 3    // and this many runs (per session for reruns)
	learnBigTokens   = 1500 // output no optimizer shortened, worth a note
	learnMaxLessons  = 10

	learnStart = "<!-- gascoigne:learned:start -->"
	learnEnd   = "<!-- gascoigne:learned:end -->"
)

// A lesson is one habit with the evidence for it and the line to teach.
type lesson struct {
	Rule     string `json:"rule"` // rerun or big
	Command  string `json:"command"`
	Sessions int    `json:"sessions"`
	Runs     int    `json:"runs"`
	Line     string `json:"line"`
}

// lessons finds the habits in entries (newest first, as the ledger returns them).
func lessons(entries []ledger.Entry) []lesson {
	type tally struct {
		runs     int
		sessions map[string]bool
		tokens   int
	}
	reruns, bigs := map[string]*tally{}, map[string]*tally{}
	add := func(m map[string]*tally, cmd, session string, tokens int) {
		t := m[cmd]
		if t == nil {
			t = &tally{sessions: map[string]bool{}}
			m[cmd] = t
		}
		t.runs++
		t.tokens += tokens
		t.sessions[session] = true
	}
	perSession := map[[2]string]int{} // session, key → shortened runs
	for _, e := range entries {
		switch {
		case e.Command == "" || e.Session == "":
		case (e.Outcome == component.Optimized || e.Outcome == component.Rerequested) && e.RawID != "" && e.Route != "fold":
			k := [2]string{e.Session, e.Key}
			if perSession[k]++; perSession[k] >= learnMinRuns {
				add(reruns, e.Command, e.Session, 0)
			}
		case e.Outcome == component.Unchanged && e.RawTokens >= learnBigTokens:
			add(bigs, e.Command, e.Session, e.RawTokens)
		}
	}
	var out []lesson
	for cmd, t := range reruns {
		if len(t.sessions) >= learnMinSessions {
			out = append(out, lesson{"rerun", cmd, len(t.sessions), t.runs, fmt.Sprintf(
				"- `%s` output arrives shortened with a hint: read the rest with `gascoigne raw <id> -grep PATTERN` instead of running it again.", cmd)})
		}
	}
	for cmd, t := range bigs {
		if len(t.sessions) >= learnMinSessions && t.runs >= learnMinRuns {
			out = append(out, lesson{"big", cmd, len(t.sessions), t.runs, fmt.Sprintf(
				"- `%s` prints about %d tokens that gascoigne cannot shorten: narrow it first (arguments, `| head`, `rg`).", cmd, t.tokens/t.runs/100*100)})
		}
	}
	slices.SortFunc(out, func(a, b lesson) int {
		return cmp.Or(cmp.Compare(b.Runs, a.Runs), cmp.Compare(a.Command, b.Command), cmp.Compare(a.Rule, b.Rule))
	})
	return out[:min(len(out), learnMaxLessons)]
}

// learnBlock is the marked block for an instruction file.
func learnBlock(ls []lesson) string {
	var b strings.Builder
	b.WriteString(learnStart + "\n## Learned from past sessions (gascoigne)\n")
	for _, l := range ls {
		b.WriteString(l.Line + "\n")
	}
	b.WriteString(learnEnd + "\n")
	return b.String()
}

// writeLearned puts the block into file, replacing an earlier one; ls empty
// removes it. The rest of the file stays; the file is deleted when nothing
// else is in it.
func writeLearned(file string, ls []lesson) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	text, had := cutBlock(string(data), learnStart, learnEnd)
	if len(ls) == 0 {
		switch {
		case !had:
			return "", nil
		case strings.TrimSpace(text) == "":
			return display(file) + ": deleted (held only the learned lines)", os.Remove(file)
		}
		return display(file) + ": removed the learned lines", os.WriteFile(file, []byte(text), 0o644)
	}
	if text = strings.TrimRight(text, "\n"); text != "" {
		text += "\n\n"
	}
	if err := os.WriteFile(file, []byte(text+learnBlock(ls)), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: wrote %d learned line(s)", display(file), len(ls)), nil
}

// cmdLearn proposes lines from the ledger. Without flags it only prints.
func cmdLearn(ctx context.Context, args []string) error {
	var (
		since  string
		limit  int
		target string
		apply  bool
		remove bool
	)
	return withLedger(ctx, "learn", args, 0, func(fs *flag.FlagSet) {
		fs.StringVar(&since, "since", "30d", "only the last `period`: 24h, 7d, 4w, 6mo")
		fs.IntVar(&limit, "limit", 5000, "newest runs to look at (`n`)")
		fs.StringVar(&target, "target", "CLAUDE.local.md", "instruction `file` for -apply and -remove (CLAUDE.md, AGENTS.md, GEMINI.md)")
		fs.BoolVar(&apply, "apply", false, "write the lines to the target as a marked block")
		fs.BoolVar(&remove, "remove", false, "take the block out of the target")
	}, func(l *ledger.Ledger, c *common, _ []string) error {
		if remove {
			d, err := writeLearned(target, nil)
			if d == "" && err == nil {
				d = display(target) + ": no learned lines"
			}
			fmt.Println(d)
			return err
		}
		q := component.StatsQuery{}
		if since != "" {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			q.Since = time.Now().Add(-d)
		}
		entries, err := l.Page(ctx, q, ledger.Page{Limit: limit})
		if err != nil {
			return err
		}
		slices.Reverse(entries) // oldest first
		ls := lessons(entries)
		if len(ls) == 0 {
			fmt.Printf("nothing learned from %d runs: a habit needs %d sessions\n", len(entries), learnMinSessions)
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "habit\tcommand\tsessions\truns")
		for _, x := range ls {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", x.Rule, x.Command, x.Sessions, x.Runs)
		}
		w.Flush()
		if !apply {
			fmt.Printf("\nlines for %s (-apply writes them):\n%s", target, learnBlock(ls))
			return nil
		}
		d, err := writeLearned(target, ls)
		fmt.Println(d)
		return err
	})
}
