package main

import (
	"cmp"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

// The MCP review: which MCP servers add tokens to the agent's tool list and
// which of them earn it. It reads two things the ledger keeps for two weeks:
// the table mcp-proxy fills (definition sizes and calls) and the hook's runs
// named mcp__server__tool (calls to servers that stay direct, no sizes).

const (
	// observeDays is how many days of data (the first one counts) a server needs
	// before the review gives a verdict: one day of silence says nothing, a
	// second day does. The review page can end the wait early (learn now).
	observeDays = 2
	// staleDays: a proxied server not listed for this long is probably removed
	// or its agent unused; it must not be called unused, nor trigger a nudge.
	staleDays = 3
	// unusedDays is the silence that makes a server or a tool unused. It is the
	// window of the Calls7 counters: change both together.
	unusedDays = 7
	// rarelyCalls and rarelyTokens: a server with at most this many calls in
	// a week whose definitions are at least this many tokens of the tool list.
	rarelyCalls  = 2
	rarelyTokens = 1500
)

// Verdicts of a server.
const (
	verdictStale    = "not seen lately" // listed last more than staleDays ago: probably no longer configured
	verdictLearning = "learning"        // seen on fewer than observeDays days
	verdictUnused   = "unused"
	verdictRarely   = "rarely used"
	verdictUsed     = "used"
)

// mcpTool is one tool of a server in the review.
type mcpTool struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"` // definition size, 0 when unknown
	Lazy   bool   `json:"lazy"`   // offered on request only: Tokens is what listing it would cost
	Calls7 int    `json:"calls_7d"`
	Calls  int    `json:"calls_14d"`
}

// mcpServer is one server in the review.
type mcpServer struct {
	ID          string                  `json:"id"`    // name#hash of the command line for proxied servers: the key of its stats
	Name        string                  `json:"name"`  // what the server reports (a proxied one) or the agent's key for it (a direct one)
	Label       string                  `json:"label"` // Name, with the id's hash when two servers share it
	Verdict     string                  `json:"verdict"`
	Sized       bool                    `json:"sized"`       // false: only calls are known (the server stays direct)
	Tokens      int                     `json:"tokens"`      // what its definitions add to the tool list
	IdleToks    int                     `json:"idle_tokens"` // of those, tools not called in unusedDays
	LazyToks    int                     `json:"lazy_tokens"` // what the tools offered on request only would cost if listed
	Calls7      int                     `json:"calls_7d"`
	CallsPrev   int                     `json:"calls_prev_7d"` // the 7 days before
	LastUsed    string                  `json:"last_used,omitempty"`
	Seen        int                     `json:"seen_days"`
	Listed      int                     `json:"listed_days_ago"` // days since a proxy last listed its tools; -1 when none did
	Agents      []string                `json:"agents"`
	Config      []mcpConfigEntry        `json:"config"`      // where the agent's config starts it, found by the command hash
	Rules       []mcpproxyreceiver.Rule `json:"rules"`       // manual rules set on the page; they take effect from the next session
	Chosen      []string                `json:"lazy_chosen"` // tools the user chose to load on request: they take effect from the next session
	Suggestions []suggestion            `json:"suggestions"` // what the usage supports changing (see mcpsuggest.go)
	Tools       []mcpTool               `json:"tools"`
}

// mcpReview is the whole review.
type mcpReview struct {
	Days    int         `json:"days"` // days of data
	Servers []mcpServer `json:"servers"`
	Notes   []string    `json:"notes"`
}

// buildReview turns the ledger's rows into the review as of now. Days are UTC.
// learned are the server ids judged before observeDays.
func buildReview(stats []ledger.ToolStat, cmds []ledger.CommandDay, now time.Time, learned map[string]bool) mcpReview {
	builder := &reviewBuilder{
		today:    now.UTC().Truncate(24 * time.Hour),
		servers:  map[string]*mcpServer{},
		tools:    map[reviewKey]*mcpTool{},
		oldest:   map[string]int{},
		listedAt: map[string]int{},
		latest:   map[reviewKey]string{},
		listedBy: map[string]map[string]bool{},
	}
	for _, row := range stats {
		builder.addStat(row)
	}
	builder.addHookCalls(cmds)
	return builder.review(learned)
}

type reviewKey struct{ server, tool string }

// reviewBuilder collects the ledger's rows per server and tool before the
// review is judged.
type reviewBuilder struct {
	today      time.Time
	servers    map[string]*mcpServer
	tools      map[reviewKey]*mcpTool
	oldest     map[string]int             // server -> age of its first row
	listedAt   map[string]int             // server -> age of its newest listing
	latest     map[reviewKey]string       // tool -> day of the size kept
	listedBy   map[string]map[string]bool // proxied server -> the tools it lists
	dataOldest int
}

// age is how many days ago day was, 0 for today.
func (builder *reviewBuilder) age(day string) int {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 1 << 20
	}
	return int(builder.today.Sub(parsed).Hours() / 24)
}

func (builder *reviewBuilder) touch(server, agent, day string) *mcpServer {
	entry := builder.servers[server]
	if entry == nil {
		name, _, _ := strings.Cut(server, "#")
		entry = &mcpServer{ID: server, Name: name, Label: name, Agents: []string{}, Tools: []mcpTool{}}
		builder.servers[server] = entry
	}
	if agent != "" && !slices.Contains(entry.Agents, agent) {
		entry.Agents = append(entry.Agents, agent)
	}
	days := builder.age(day)
	builder.oldest[server] = max(builder.oldest[server], days)
	builder.dataOldest = max(builder.dataOldest, days)
	return entry
}

func (builder *reviewBuilder) tool(server, name string) *mcpTool {
	entry := builder.tools[reviewKey{server, name}]
	if entry == nil {
		entry = &mcpTool{Name: name}
		builder.tools[reviewKey{server, name}] = entry
	}
	return entry
}

func (builder *reviewBuilder) count(server *mcpServer, tool *mcpTool, day string, calls int) {
	days := builder.age(day)
	switch {
	case days < 7:
		server.Calls7 += calls
		tool.Calls7 += calls
	case days < 14:
		server.CallsPrev += calls
	}
	if days < 14 {
		tool.Calls += calls
	}
	if calls > 0 && (server.LastUsed == "" || day > server.LastUsed) {
		server.LastUsed = day
	}
}

// addStat adds one row of the proxy's table: a tool's size and calls.
func (builder *reviewBuilder) addStat(row ledger.ToolStat) {
	server := builder.touch(row.Server, row.Agent, row.Day)
	tool := builder.tool(row.Server, row.Tool)
	if row.Tokens > 0 {
		if days, ok := builder.listedAt[row.Server]; !ok || builder.age(row.Day) < days {
			builder.listedAt[row.Server] = builder.age(row.Day)
		}
		key := reviewKey{row.Server, row.Tool}
		if row.Day >= builder.latest[key] { // the last listing's size
			tool.Tokens, tool.Lazy, builder.latest[key] = row.Tokens, row.Lazy, row.Day
		}
		server.Sized = true
	}
	if builder.listedBy[row.Server] == nil {
		builder.listedBy[row.Server] = map[string]bool{}
	}
	builder.listedBy[row.Server][row.Tool] = true
	builder.count(server, tool, row.Day, row.Calls)
}

// addHookCalls adds the hook's runs of mcp__server__tool, for the servers
// mcp-proxy did not already count.
func (builder *reviewBuilder) addHookCalls(cmds []ledger.CommandDay) {
	covered := coveredByProxy(cmds, builder.listedBy)
	for _, cmd := range cmds {
		server, name, ok := splitMCPName(cmd.Command)
		if !ok || covered[server] {
			continue
		}
		entry := builder.touch(server, cmd.Agent, cmd.Day)
		builder.count(entry, builder.tool(server, name), cmd.Day, cmd.Runs)
	}
}

// review judges every server collected and orders them by use.
func (builder *reviewBuilder) review(learned map[string]bool) mcpReview {
	for key, tool := range builder.tools {
		server := builder.servers[key.server]
		server.Tools = append(server.Tools, *tool)
	}
	review := mcpReview{Days: builder.dataOldest + 1, Servers: []mcpServer{}}
	for _, server := range builder.servers {
		builder.settle(server)
		server.Verdict = verdict(server, learned[server.ID])
		server.Suggestions = suggestMCP(*server)
		review.Servers = append(review.Servers, *server)
	}
	labelDuplicates(review.Servers)
	slices.SortFunc(review.Servers, func(left, right mcpServer) int {
		return cmp.Or(cmp.Compare(right.Calls7, left.Calls7), cmp.Compare(right.Tokens, left.Tokens), cmp.Compare(left.Name, right.Name))
	})
	review.Notes = reviewNotes(review)
	return review
}

// settle orders a server's tools and sums their sizes.
func (builder *reviewBuilder) settle(server *mcpServer) {
	slices.SortFunc(server.Tools, func(left, right mcpTool) int { // the most used first
		return cmp.Or(cmp.Compare(right.Calls7, left.Calls7), cmp.Compare(right.Calls, left.Calls), cmp.Compare(right.Tokens, left.Tokens), cmp.Compare(left.Name, right.Name))
	})
	slices.Sort(server.Agents)
	server.Seen = builder.oldest[server.ID] + 1
	server.Listed = -1
	if days, ok := builder.listedAt[server.ID]; ok {
		server.Listed = days
	}
	for _, tool := range server.Tools {
		if tool.Lazy {
			server.LazyToks += tool.Tokens
			continue
		}
		server.Tokens += tool.Tokens
		if tool.Calls7 == 0 && !lazyModeTool(tool.Name) { // the lazy-mode tools are the price of lazy tools, never idle
			server.IdleToks += tool.Tokens
		}
	}
}

// verdict judges a server; learned (the user's "learn now") skips the wait
// for observeDays.
func verdict(server *mcpServer, learned bool) string {
	calls := server.Calls7 // the last unusedDays
	switch {
	case server.Sized && server.Listed >= staleDays:
		return verdictStale
	case server.Seen < observeDays && !learned:
		return verdictLearning
	case calls == 0:
		return verdictUnused
	case server.Sized && server.Calls7 <= rarelyCalls && server.Tokens >= rarelyTokens:
		return verdictRarely
	}
	return verdictUsed
}

// coveredByProxy names the servers (by the agent's key, as the hook sees them)
// whose calls mcp-proxy already counted: every tool the hook saw them call is a
// tool one proxied server lists. A direct server that shares a common tool name
// (search, query) with a proxied one is not covered unless all of its tools are.
func coveredByProxy(cmds []ledger.CommandDay, listedBy map[string]map[string]bool) map[string]bool {
	seen := map[string]map[string]bool{}
	for _, c := range cmds {
		if server, name, ok := splitMCPName(c.Command); ok {
			if seen[server] == nil {
				seen[server] = map[string]bool{}
			}
			seen[server][name] = true
		}
	}
	covered := map[string]bool{}
	for server, tools := range seen {
		for _, listed := range listedBy {
			if allIn(tools, listed) {
				covered[server] = true
				break
			}
		}
	}
	return covered
}

func allIn(sub, set map[string]bool) bool {
	for k := range sub {
		if !set[k] {
			return false
		}
	}
	return true
}

// lazyModeTool is one of the two tools mcp-proxy adds to reach lazy tools.
func lazyModeTool(name string) bool { return name == "search_tools" || name == "call_tool" }

// splitMCPName splits mcp__server__tool (Claude and Codex naming).
func splitMCPName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return "", "", false
	}
	server, tool, ok = strings.Cut(rest, "__")
	if !ok || server == "" || tool == "" || server == "gascoigne" {
		return "", "", false
	}
	if lazyModeTool(tool) { // mcp-proxy's lazy tools: the real call is counted there
		return "", "", false
	}
	return server, tool, true
}

// reviewNotes says what the numbers cannot show.
func reviewNotes(review mcpReview) []string {
	notes := []string{}
	if len(review.Servers) == 0 {
		return []string{"No MCP tool data yet. Servers started through gascoigne mcp-proxy report their tools and calls; other servers show up here once the agent calls them."}
	}
	if review.Days < observeDays {
		notes = append(notes, fmt.Sprintf("Only %d day(s) of data: no server is called unused before %d days of observation.", review.Days, observeDays))
	}
	for _, s := range review.Servers {
		if !s.Sized {
			notes = append(notes, "Servers without a size stay direct (they do not go through mcp-proxy): only their calls are known, and a direct server that was never called does not appear. gascoigne install -g routes stdio servers through the proxy.")
			break
		}
	}
	return notes
}

// printReview writes the review as a table, the terminal form of the page.
func printReview(writer io.Writer, review mcpReview) {
	if len(review.Servers) == 0 {
		fmt.Fprintln(writer, review.Notes[0])
		return
	}
	tw := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVER\tVERDICT\tTOKENS/REQ\tIDLE TOKENS\tCALLS 7D\tPREV 7D\tLAST USED")
	for _, s := range review.Servers {
		size, idle := "?", "?"
		if s.Sized {
			size, idle = fmt.Sprint(s.Tokens), fmt.Sprint(s.IdleToks)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n", s.Name, s.Verdict, size, idle, s.Calls7, s.CallsPrev, cmp.Or(s.LastUsed, "-"))
	}
	tw.Flush()
	printSuggestions(writer, review)
	for _, n := range review.Notes {
		fmt.Fprintln(writer, "\n"+n)
	}
}

// loadReview reads the last two weeks from the ledger.
func loadReview(ctx context.Context, store *ledger.Ledger, now time.Time, chosen map[string][]string, learned []string) (mcpReview, error) {
	rules := readPrefs().MCPRules
	since := now.AddDate(0, 0, -2*7)
	stats, err := store.ToolStats(ctx, since)
	if err != nil {
		return mcpReview{}, err
	}
	cmds, err := store.MCPCommands(ctx, since)
	if err != nil {
		return mcpReview{}, err
	}
	review := buildReview(stats, cmds, now, idSet(learned))
	home := home()
	configs := proxiedConfigs(home, projectDirs())
	for i, s := range review.Servers {
		review.Servers[i].Config = []mcpConfigEntry{}
		if _, hash, ok := strings.Cut(s.ID, "#"); ok {
			review.Servers[i].Config = append(review.Servers[i].Config, configs[hash]...)
		}
		review.Servers[i].Rules = append([]mcpproxyreceiver.Rule{}, rules[s.ID]...)
		review.Servers[i].Chosen = []string{}
		if c := chosen[s.ID]; len(c) > 0 {
			review.Servers[i].Chosen = c
		}
		review.Servers[i].Suggestions = suggestMCP(review.Servers[i])
	}
	return review, nil
}

// cmdMCPReview opens the review page; the table is the fallback for a
// terminal without a browser.
func cmdMCPReview(ctx context.Context, args []string) error {
	var asJSON, text, learn, optimize bool
	open := true
	return withLedger(ctx, "mcp-review", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&asJSON, "json", false, "print the review as JSON")
		fs.BoolVar(&text, "text", false, "print the table instead of opening the page")
		fs.BoolVar(&learn, "learn", false, "judge the servers still learning now, without waiting for the observation days")
		fs.BoolVar(&optimize, "optimize", false, "apply the suggestions: load the tools nothing called on request")
		fs.BoolVar(&open, "open", true, "open the page in a browser; false only prints its address")
	}, func(store *ledger.Ledger, _ *common, _ []string) error {
		if learn {
			if _, err := learnServers(ctx, store, ""); err != nil {
				return err
			}
		}
		if optimize {
			_, applied, err := optimizeServers(ctx, store, "")
			if err != nil {
				return err
			}
			fmt.Printf("%d tool(s) load on request from the next session, %d tokens out of the tool list\n", len(applied), savedBy(applied))
		}
		if !asJSON && !text && canBrowse(ctx) {
			link := "http://" + serverAddr() + "/mcp"
			fmt.Println(link)
			if open {
				openBrowser(link)
			}
			return nil
		}
		review, err := loadReview(ctx, store, time.Now(), lazyChoices(), readPrefs().LearnedMCP)
		if err != nil {
			return err
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(review)
		}
		printReview(os.Stdout, review)
		return nil
	})
}

// canBrowse reports whether the review page can be shown: a display, no CI,
// and the background server answering.
func canBrowse(ctx context.Context) bool {
	return !serverOff() && os.Getenv("CI") == "" && hasDisplay(runtime.GOOS) && serverUp(ctx) == nil
}

// labelDuplicates adds the command hash to servers that share a name.
func labelDuplicates(servers []mcpServer) {
	count := map[string]int{}
	for _, s := range servers {
		count[s.Name]++
	}
	for i, s := range servers {
		if _, hash, ok := strings.Cut(s.ID, "#"); ok && count[s.Name] > 1 {
			servers[i].Label = s.Name + " (" + hash + ")"
		}
	}
}

// savedBy sums what suggestions take out of the tool list.
func savedBy(sugs []suggestion) (n int) {
	for _, s := range sugs {
		n += s.Saves
	}
	return n
}

// printSuggestions lists what the usage supports changing, per server: the
// tools to load on request in one line, each piece of advice in its own.
func printSuggestions(writer io.Writer, request mcpReview) {
	first := true
	for _, s := range request.Servers {
		var lazy []string
		var advice []suggestion
		for _, sg := range s.Suggestions {
			if sg.applies() {
				lazy = append(lazy, sg.Tool)
			} else {
				advice = append(advice, sg)
			}
		}
		if len(lazy)+len(advice) == 0 {
			continue
		}
		if first {
			fmt.Fprintln(writer, "\nSUGGESTIONS (mcp-review -optimize loads the listed tools on request)")
			first = false
		}
		if len(lazy) > 0 {
			fmt.Fprintf(writer, "  %s: load on request %s (%d tokens)\n", s.Name, strings.Join(lazy, ", "), savedBy(s.Suggestions))
		}
		for _, sg := range advice {
			fmt.Fprintf(writer, "  %s: %s\n", s.Name, sg.Reason)
		}
	}
}
