package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// readReplayReport loads an earlier replay -json report; "" gives nil.
func readReplayReport(path string) (*replayReport, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report *replayReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return report, nil
}

type replayReport struct {
	Sessions int `json:"sessions"`
	Requests int `json:"requests"`
	// Compared counts messages a later request sent again unchanged by the
	// client; Unstable those the proxy sent differently the second time.
	Compared int                    `json:"compared"`
	Unstable int                    `json:"unstable"`
	Examples []string               `json:"unstable_examples,omitempty"`
	Tools    map[string]*toolReplay `json:"tools"`
	// Message input cost of every request as the client sent it and as
	// the proxy forwarded it (promptCost); system prompt and tools are the
	// same on both sides and left out.
	CostRaw float64 `json:"cost_raw_usd"`
	CostOut float64 `json:"cost_out_usd"`
	// Assumed lists sessions priced with the default model: their
	// transcript names none cost knows.
	Assumed int `json:"assumed_model_sessions,omitempty"`
}

type toolReplay struct {
	Results int `json:"results"`
	Raw     int `json:"raw_tokens"`
	Out     int `json:"out_tokens"`
	Folded  int `json:"folded"`
}

// addResults adds the session's latest measure of every tool result to the
// per-tool totals.
func (rep *replayReport) addResults(sess *session) {
	for id, result := range sess.results {
		tool := rep.Tools[cmp.Or(sess.tools[id], "?")]
		if tool == nil {
			tool = &toolReplay{}
			rep.Tools[cmp.Or(sess.tools[id], "?")] = tool
		}
		tool.Results++
		tool.Raw += result.raw
		tool.Out += result.out
		if result.folded {
			tool.Folded++
		}
	}
}

func (r replayReport) print() {
	fmt.Printf("replayed %d sessions, %d requests\n", r.Sessions, r.Requests)
	fmt.Printf("prompt cache: %d of %d messages sent again were byte-identical (%s)\n",
		r.Compared-r.Unstable, r.Compared, mcptool.Percent(r.Compared-r.Unstable, r.Compared))
	for _, e := range r.Examples {
		fmt.Println("  changed:", e)
	}
	fmt.Printf("message input cost, cache reads and writes priced: $%.2f → $%.2f (%s saved)\n",
		r.CostRaw, r.CostOut, mcptool.Percent(int((r.CostRaw-r.CostOut)*1e4), int(r.CostRaw*1e4)))
	if r.Assumed > 0 {
		fmt.Printf("  %d sessions name no known model: priced as %s\n", r.Assumed, cost.AgentModels["claude"])
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "tool\tresults\traw tok\tout tok\tsaved\tfolded")
	var total toolReplay
	names := slices.SortedFunc(maps.Keys(r.Tools), func(left, right string) int {
		return cmp.Or(cmp.Compare(r.Tools[right].Raw-r.Tools[right].Out, r.Tools[left].Raw-r.Tools[left].Out), cmp.Compare(left, right))
	})
	for _, n := range names {
		t := r.Tools[n]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%d\n", n, t.Results, t.Raw, t.Out, mcptool.Percent(t.Raw-t.Out, t.Raw), t.Folded)
		total.Results += t.Results
		total.Raw += t.Raw
		total.Out += t.Out
		total.Folded += t.Folded
	}
	fmt.Fprintf(w, "total\t%d\t%d\t%d\t%s\t%d\n", total.Results, total.Raw, total.Out, mcptool.Percent(total.Raw-total.Out, total.Raw), total.Folded)
	w.Flush()
}

// compare prints what changed since an earlier report of the same transcripts.
func (r replayReport) compare(old *replayReport) {
	fmt.Printf("since the earlier report: requests %d → %d, unstable messages %d → %d, message input $%.2f → $%.2f (%+.2f)\n",
		old.Requests, r.Requests, old.Unstable, r.Unstable, old.CostOut, r.CostOut, r.CostOut-old.CostOut)
	changes := toolChanges(old.Tools, r.Tools)
	if len(changes) == 0 {
		fmt.Println("no tool's results changed")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "tool\tout tok before\tout tok now\tchange")
	for _, c := range changes {
		fmt.Fprintf(w, "%s\t%d\t%d\t%+d\n", c.tool, c.was, c.now, c.now-c.was)
	}
	w.Flush()
}

type toolChange struct {
	tool     string
	was, now int
}

// toolChanges lists tools whose forwarded tokens differ, by name; a tool
// one side lacks counts as 0.
func toolChanges(old, cur map[string]*toolReplay) []toolChange {
	all := maps.Clone(cur)
	maps.Insert(all, maps.All(old))
	var out []toolChange
	for _, n := range slices.Sorted(maps.Keys(all)) {
		var c toolChange
		c.tool = n
		if t := old[n]; t != nil {
			c.was = t.Out
		}
		if t := cur[n]; t != nil {
			c.now = t.Out
		}
		if c.was != c.now {
			out = append(out, c)
		}
	}
	return out
}
