// Package mcptool holds MCP pieces shared by MCP receivers: the retrieve and
// stats tools and the hint appended to optimized results.
package mcptool

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/cost"
	"github.com/lukaszb/gascoigne/payload"
)

// Ledger looks up the ledger extension by id.
func Ledger(host component.Host, id string) (component.Ledger, error) {
	pid, err := component.ParseID(id)
	if err != nil {
		return nil, err
	}
	ext, ok := host.Extension(pid)
	if !ok {
		return nil, fmt.Errorf("extension %s not configured", pid)
	}
	l, ok := ext.(component.Ledger)
	if !ok {
		return nil, fmt.Errorf("extension %s is not a ledger", pid)
	}
	return l, nil
}

// Text returns the payload text plus, when optimized, how to get the original.
func Text(p *payload.Payload, retrieveTool string) string {
	text := p.Text()
	id := p.Attrs[payload.AttrRawID]
	if id == "" {
		return text
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fmt.Sprintf("%s[optimized %s→%s tokens · full: %s id=%s]", text,
		p.Attrs[payload.AttrRawTokens], p.Attrs[payload.AttrOutTokens], retrieveTool, id)
}

func Result(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

type RetrieveInput struct {
	ID string `json:"id" jsonschema:"id from an [optimized ... id=<id>] hint"`
}

// AddRetrieve registers a tool returning original, unoptimized content.
// Asking for it marks the source as badly optimized, so next time it is not.
func AddRetrieve(s *mcp.Server, name string, l component.Ledger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Return the full original content of an optimized result, by the id from its [optimized ...] hint.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RetrieveInput) (*mcp.CallToolResult, any, error) {
		key, p, err := l.GetRaw(ctx, in.ID)
		if err != nil {
			return nil, nil, err
		}
		if key != "" {
			if err := l.MarkFailure(ctx, key, component.ReasonRawRequested); err != nil {
				return nil, nil, err
			}
		}
		return Result(p.Text()), nil, nil
	})
}

type StatsInput struct {
	By      string `json:"by,omitempty" jsonschema:"one grouping (command, optimizer, family, outcome, reason, session, route); default: overview"`
	Session string `json:"session,omitempty" jsonschema:"session id; default this session, 'all' for every session"`
	Format  string `json:"format,omitempty" jsonschema:"table (default) or json"`
}

// AddStats registers a tool reporting token savings, by default per optimizer
// family in the calling session.
func AddStats(s *mcp.Server, l component.Ledger, sessions *Sessions) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stats",
		Description: "Token savings so far (this session by default): overview of outcomes, commands, optimizers and skips, or one grouping.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StatsInput) (*mcp.CallToolResult, any, error) {
		q := component.StatsQuery{By: in.By, Session: in.Session}
		switch q.Session {
		case "":
			q.Session = sessions.Of(req)
		case "all":
			q.Session = ""
		}
		if q.By == "" {
			o, err := BuildOverview(ctx, l, q)
			if err != nil {
				return nil, nil, err
			}
			if in.Format == "json" {
				data, err := json.Marshal(o)
				return Result(string(data)), nil, err
			}
			return Result(o.Table()), nil, nil
		}
		stats, err := l.Stats(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		if in.Format == "json" {
			data, err := json.Marshal(Report(q, stats))
			return Result(string(data)), nil, err
		}
		return Result(StatsTable(q.By, stats)), nil, nil
	})
}

type identityKey struct{}

// Identity is who tool calls run for; empty fields are resolved per call.
type Identity struct{ Session, Agent, Model string }

// WithIdentity sets explicit identity (e.g. from CLI flags) for MCP servers run with ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// Sessions resolves the identity of tool calls: explicit values from the
// context first; else the MCP transport's session id (or one generated per
// server run) and the client's name as agent.
type Sessions struct {
	explicit  Identity
	generated string
}

func NewSessions(ctx context.Context) *Sessions {
	explicit, _ := ctx.Value(identityKey{}).(Identity)
	return &Sessions{explicit: explicit, generated: fmt.Sprintf("mcp-%s-%04x", time.Now().Format("20060102-150405"), rand.N(1<<16))}
}

func (s *Sessions) Of(req *mcp.CallToolRequest) string {
	if s.explicit.Session != "" {
		return s.explicit.Session
	}
	if req != nil && req.Session != nil && req.Session.ID() != "" {
		return req.Session.ID()
	}
	return s.generated
}

// Agent is the explicit agent, else the MCP client's name.
func (s *Sessions) Agent(req *mcp.CallToolRequest) string {
	if s.explicit.Agent != "" || req == nil {
		return s.explicit.Agent
	}
	if info := req.ClientInfo(); info != nil {
		return info.Name
	}
	return ""
}

// Tag sets session, agent and model attrs on p for a tool call.
func (s *Sessions) Tag(p *payload.Payload, req *mcp.CallToolRequest) {
	p.Set(payload.AttrSession, s.Of(req))
	p.Set(payload.AttrAgent, s.Agent(req))
	p.Set(payload.AttrModel, s.explicit.Model)
}

// StatsReport is the JSON form of a stats query.
type StatsReport struct {
	By      string      `json:"by"`
	Session string      `json:"session,omitempty"`
	Groups  []StatsJSON `json:"groups"`
	Total   StatsJSON   `json:"total"`
}

type StatsJSON struct {
	Group     string     `json:"group"`
	Family    string     `json:"family,omitempty"`
	Command   string     `json:"command,omitempty"`
	Outcome   string     `json:"outcome,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Runs      int        `json:"runs"` // steps for family/optimizer groupings
	Optimized int        `json:"optimized"`
	Failed    int        `json:"failed"`
	RawTokens int        `json:"raw_tokens"`
	OutTokens int        `json:"out_tokens"`
	Saved     int        `json:"saved_tokens"`
	SavedPct  float64    `json:"saved_pct"`
	First     *time.Time `json:"first,omitempty"`
	Last      *time.Time `json:"last,omitempty"`
}

// Report builds the JSON form of stats, with totals.
func Report(q component.StatsQuery, stats []component.Stat) StatsReport {
	r := StatsReport{By: cmp.Or(q.By, component.ByRoute), Session: q.Session, Groups: []StatsJSON{}}
	total := component.Stat{Group: "total"}
	for _, s := range stats {
		j := statJSON(s)
		j.First, j.Last = &s.First, &s.Last
		r.Groups = append(r.Groups, j)
		total = add(total, s)
	}
	r.Total = statJSON(total)
	return r
}

func statJSON(s component.Stat) StatsJSON {
	pct := 0.0
	if s.RawTokens > 0 {
		pct = math.Round(1000*float64(s.Saved())/float64(s.RawTokens)) / 10
	}
	return StatsJSON{Group: s.Group, Family: s.Family, Command: s.Command, Outcome: s.Outcome, Reason: s.Reason, Runs: s.Runs, Optimized: s.Optimized, Failed: s.Failed,
		RawTokens: s.RawTokens, OutTokens: s.OutTokens, Saved: s.Saved(), SavedPct: pct}
}

func add(a, b component.Stat) component.Stat {
	a.Runs += b.Runs
	a.Optimized += b.Optimized
	a.Failed += b.Failed
	a.RawTokens += b.RawTokens
	a.OutTokens += b.OutTokens
	return a
}

// StatsTable renders stats with a total row.
func StatsTable(by string, stats []component.Stat) string {
	by = cmp.Or(by, component.ByRoute)
	steps := by == component.ByFamily || by == component.ByOptimizer || by == component.ByDialect
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	switch {
	case steps:
		fmt.Fprintf(w, "%s\tsteps\traw tok\tout tok\tsaved\tsaved %%\n", by)
	case by == component.BySession:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\tfirst\tlast\n", by)
	default:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\n", by)
	}
	total := component.Stat{Group: "total"}
	row := func(s component.Stat) {
		saved := fmt.Sprintf("%d\t%d\t%d\t%s", s.RawTokens, s.OutTokens, s.Saved(), Percent(s.Saved(), s.RawTokens))
		switch {
		case steps:
			fmt.Fprintf(w, "%s\t%d\t%s\n", s.Group, s.Runs, saved)
		case by == component.BySession && s.Group != "total":
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\t%s\n", s.Group, s.Runs, s.Optimized, s.Failed, saved,
				s.First.Format(time.DateTime), s.Last.Format(time.DateTime))
		default:
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", s.Group, s.Runs, s.Optimized, s.Failed, saved)
		}
	}
	for _, s := range stats {
		row(s)
		total = add(total, s)
	}
	row(total)
	w.Flush()
	return b.String()
}

// Percent formats part/whole, "-" when whole is 0.
func Percent(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(part)/float64(whole))
}

// Overview answers "where do savings come from": totals, per command, per
// optimizer and why runs were skipped.
type Overview struct {
	Session    string               `json:"session,omitempty"`
	Filter     component.StatsQuery `json:"-"`
	Total      StatsJSON            `json:"total"`
	Outcomes   []StatsJSON          `json:"outcomes"`
	Commands   []StatsJSON          `json:"commands"`
	Optimizers []StatsJSON          `json:"optimizers"`
	Dialects   []StatsJSON          `json:"dialects"`
	Agents     []StatsJSON          `json:"agents"`
	Models     []StatsJSON          `json:"models"`
	Days       []StatsJSON          `json:"days"` // last 7 days (or since filter.Since), oldest first
	Cost       Costs                `json:"cost"`
	// Runs not optimized, per stopping component (filter or optimizer) and
	// command, with the latest recorded reason.
	Skipped   []StatsJSON `json:"skipped"`
	Failed    []StatsJSON `json:"failed"`
	Unchanged []StatsJSON `json:"unchanged"`
}

func BuildOverview(ctx context.Context, l component.Ledger, filter component.StatsQuery) (Overview, error) {
	o := Overview{Session: filter.Session, Filter: filter}
	for _, part := range []struct {
		by  string
		out *[]StatsJSON
	}{
		{component.ByOutcome, &o.Outcomes},
		{component.ByCommand, &o.Commands},
		{component.ByOptimizer, &o.Optimizers},
		{component.ByDialect, &o.Dialects},
		{component.ByAgent, &o.Agents},
		{component.ByModel, &o.Models},
		{component.ByDay, &o.Days},
	} {
		q := filter
		q.By = part.by
		if part.by == component.ByDay && q.Since.IsZero() {
			y, m, d := time.Now().AddDate(0, 0, -6).Date()
			q.Since = time.Date(y, m, d, 0, 0, 0, 0, time.Local) // local midnight, 7 days incl. today
		}
		stats, err := l.Stats(ctx, q)
		if err != nil {
			return o, err
		}
		r := Report(component.StatsQuery{By: part.by}, stats)
		*part.out = r.Groups
		if part.by == component.ByDay {
			o.Days = fillDays(r.Groups, q.Since, time.Now())
		}
		if part.by == component.ByCommand {
			o.Total = r.Total
		}
	}
	costs, err := BuildCosts(ctx, l, filter, cost.AgentContext)
	if err != nil {
		return o, err
	}
	o.Cost = costs
	rq := filter
	rq.By = component.ByReason
	reasons, err := l.Stats(ctx, rq)
	if err != nil {
		return o, err
	}
	o.Skipped, o.Failed, o.Unchanged = []StatsJSON{}, []StatsJSON{}, []StatsJSON{}
	for _, s := range reasons {
		j := statJSON(s)
		if j.Reason == "" {
			typ, _, _ := strings.Cut(s.Group, "/")
			j.Reason = SkipReason(typ) // recorded before reasons were
		}
		switch component.Outcome(s.Outcome) {
		case component.Bypassed:
			o.Skipped = append(o.Skipped, j)
		case component.Failed:
			o.Failed = append(o.Failed, j)
		default:
			o.Unchanged = append(o.Unchanged, j)
		}
	}
	return o, nil
}

// outcomeNotes say what each outcome means for tokens.
var outcomeNotes = map[string]string{
	string(component.Optimized): "shrunk; original retrievable",
	string(component.Unchanged): "no optimizer applied",
	string(component.Bypassed):  "passed through by a filter (see skipped)",
	string(component.Failed):    "optimizer error, original returned",
}

// skipReasons describe filter types, for runs recorded before reasons were.
var skipReasons = map[string]string{
	"minsize": "output below min_tokens",
	"bypass":  "raw output requested",
	"history": "source optimized badly before",
}

// SkipReason explains why a filter type bypasses optimization.
func SkipReason(filterType string) string {
	return cmp.Or(skipReasons[filterType], "-")
}

// Table renders the overview as plain text.
func (o Overview) Table() string {
	var b strings.Builder
	if o.Session != "" {
		fmt.Fprintf(&b, "session %s\n", o.Session)
	}
	t := o.Total
	fmt.Fprintf(&b, "%d runs · %d optimized · %d skipped · %d failed · %d → %d tokens · saved %d (%s)\n",
		t.Runs, t.Optimized, sumRuns(o.Skipped), t.Failed, t.RawTokens, t.OutTokens, t.Saved, Percent(t.Saved, t.RawTokens))
	if c := o.Cost; len(c.Savings) > 0 {
		fmt.Fprintf(&b, "worth $%.2f at first read, $%.2f while in context (%.0f rereads)\n", c.FirstRead, c.InContext, c.Rereads)
	}

	section := func(title, header string, rows []StatsJSON, row func(*tabwriter.Writer, StatsJSON)) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n%s\n", title)
		w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, header)
		for _, r := range rows {
			row(w, r)
		}
		w.Flush()
	}
	section("by outcome", "outcome\truns\traw tok\tout tok\tsaved\tshare of input\t", o.Outcomes, func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%s\t%s\n", r.Group, r.Runs, r.RawTokens, r.OutTokens, r.Saved, Percent(r.RawTokens, t.RawTokens), outcomeNotes[r.Group])
	})
	section("by command", "command\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", o.Commands, func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", r.Group, r.Runs, r.Optimized, r.RawTokens, r.OutTokens, r.Saved, Percent(r.Saved, r.RawTokens))
	})
	section("by agent", "agent\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", Named(o.Agents), func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", r.Group, r.Runs, r.Optimized, r.RawTokens, r.OutTokens, r.Saved, Percent(r.Saved, r.RawTokens))
	})
	section("by model", "model\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", Named(o.Models), func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", r.Group, r.Runs, r.Optimized, r.RawTokens, r.OutTokens, r.Saved, Percent(r.Saved, r.RawTokens))
	})
	section("by optimizer", "optimizer\tfamily\tsteps\traw tok\tout tok\tsaved\tsaved %", o.Optimizers, func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%s\n", r.Group, cmp.Or(r.Family, "-"), r.Runs, r.RawTokens, r.OutTokens, r.Saved, Percent(r.Saved, r.RawTokens))
	})
	reasons := func(w *tabwriter.Writer, r StatsJSON) {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", r.Group, cmp.Or(r.Command, "-"), r.Runs, r.RawTokens, r.Reason)
	}
	section("skipped", "filter\tcommand\truns\ttokens\treason (latest)", o.Skipped, reasons)
	section("failed", "optimizer\tcommand\truns\ttokens\terror (latest)", o.Failed, reasons)
	section("unchanged", "-\tcommand\truns\ttokens\treason (latest)", o.Unchanged, reasons)
	return b.String()
}

func sumRuns(rows []StatsJSON) int {
	n := 0
	for _, r := range rows {
		n += r.Runs
	}
	return n
}

// Named returns rows unless the only group is "-" (dimension never set).
func Named(rows []StatsJSON) []StatsJSON {
	if len(rows) == 1 && rows[0].Group == "-" {
		return nil
	}
	return rows
}

// fillDays adds empty days between from and to, so trends show quiet days.
func fillDays(days []StatsJSON, from, to time.Time) []StatsJSON {
	have := map[string]StatsJSON{}
	for _, d := range days {
		have[d.Group] = d
	}
	out := []StatsJSON{}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := day.Format(time.DateOnly)
		if d, ok := have[key]; ok {
			out = append(out, d)
		} else {
			out = append(out, StatsJSON{Group: key})
		}
	}
	return out
}
