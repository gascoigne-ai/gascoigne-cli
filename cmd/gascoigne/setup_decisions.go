package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/prose"
	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

// A decision is one question gascoigne asks the user before it changes
// something beyond its own files: in the browser (/setup) or in the terminal
// (gascoigne setup). Nothing it covers is done until the user answers.
type decision struct {
	ID      string   `json:"id"`
	Group   string   `json:"group"` // permissions, updates or agents: the page's sections
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Options []option `json:"options"`
	Default string   `json:"default"`
	Current string   `json:"current"` // the saved answer, "" = not decided
	// Optional questions always have a current answer and never count as
	// open: turning an experimental feature on is up to the user.
	Optional bool `json:"optional,omitempty"`
	// Example, How and Impact explain a feature before it is turned on: what
	// it looks like, how it works, what it changes for the user.
	Example string `json:"example,omitempty"`
	How     string `json:"how,omitempty"`
	Impact  string `json:"impact,omitempty"`
}

type option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"` // what choosing it does, in a sentence
}

// decisions are the questions, with the saved answers filled in.
func decisions(settings prefs) []decision {
	ds := []decision{
		{ID: "allow", Group: "permissions", Current: settings.Allow, Default: "local",
			Title: "Fewer approval prompts",
			Text:  "Claude asks before each use of gascoigne's own tools. Allowing them adds rules to claude's settings; gascoigne unwrap removes exactly those.",
			Options: []option{
				{"local", "Allow the read-only tools", "retrieve, show, stats and compress only read what gascoigne already holds."},
				{"all", "Also allow fetch", "fetch reads web pages and APIs, so it reaches the network."},
				{"none", "Keep asking", "claude asks before every gascoigne tool call, as now."},
			}},
		{ID: "autosync", Group: "updates", Current: string(settings.Autosync), Default: string(toggleOn),
			Title: "Update hooks automatically after upgrades",
			Text:  "A new gascoigne can add or change the hooks it installed in your agents.",
			Options: []option{
				{string(toggleOn), "Keep them up to date", "At the next session start after an upgrade, in your user-level configs. Other keys stay and a backup is kept. Project files are never touched automatically."},
				{string(toggleOff), "Only when I ask", "Nothing changes until you run gascoigne upgrade."},
			}},
	}
	ds = append(ds, mcpModeDecision(settings), billingDecision(settings))
	if q, ok := updateDecision(settings); ok {
		ds = slices.Insert(ds, 1, q) // the immediate update before the standing one
	}
	ds = append(ds, releaseDecision(settings), mcpReviewDecision(settings))
	for _, name := range newAgents(settings) {
		ds = append(ds, decision{ID: integratePrefix + name, Default: "yes", Current: declinedAs(settings, name),
			Group: "agents",
			Title: "Use gascoigne with " + name,
			Text:  name + " is on this machine, but gascoigne is not connected to it, so its command output is not shortened.",
			Options: []option{
				{"yes", "Set it up for " + name, "Writes gascoigne's hooks and MCP server into your user-level " + name + " config. A backup is kept; gascoigne unwrap removes them."},
				{"no", "Not for " + name, "Nothing is installed, and you are not asked again."},
			}})
	}
	ds = append(ds, experimentDecisions(settings)...)
	return append(ds, proseModelDecision(settings))
}

// staleInstalls are the recorded installs written by an older registration.
func staleInstalls() []install {
	return slices.DeleteFunc(readInstalls(), func(entry install) bool { return entry.Registration >= registrationVersion })
}

// updateDecision asks whether to bring older installs up to this version,
// unless autosync does it by itself.
func updateDecision(settings prefs) (decision, bool) {
	stale := staleInstalls()
	if len(stale) == 0 || settings.Autosync == toggleOn {
		return decision{}, false
	}
	var which, news []string
	oldest := registrationVersion
	for _, e := range stale {
		scope := "all projects"
		if !e.Global {
			scope = e.Dir
		}
		which = append(which, e.Agent+" ("+scope+")")
		oldest = min(oldest, e.Registration)
	}
	for v := oldest + 1; v <= registrationVersion; v++ {
		if n := registrationNotes[v]; n != "" {
			news = append(news, n)
		}
	}
	text := "This gascoigne is newer than what set up " + strings.Join(which, ", ") + "."
	if len(news) > 0 {
		text += " What changed: " + strings.Join(news, "; ") + "."
	}
	current := ""
	if settings.UpdateDeclined >= registrationVersion {
		current = "no"
	}
	return decision{ID: "update", Group: "updates", Title: "Update gascoigne's hooks now", Text: text, Default: "yes", Current: current,
		Options: []option{
			{"yes", "Update them now", "Rewrites the user-level ones (other keys stay, a backup is kept). Project ones need gascoigne upgrade run in the project."},
			{"no", "Not now", "Asked again with the next version."},
		}}, true
}

// setupView is what the page shows: the machine as gascoigne found it, then
// the questions.
type setupView struct {
	Version   string        `json:"version"`
	Agents    []agentState  `json:"agents"`
	Questions []decision    `json:"questions"`
	Pricing   *pricingSetup `json:"pricing"`
}

// agentState is one agent's line in the overview.
type agentState struct {
	Name  string `json:"name"`
	Scope string `json:"scope,omitempty"` // all projects, or a project dir
	State string `json:"state"`           // connected, older, not connected or declined
}

// buildSetupView lists the recorded installs (connected, or older than this
// version) and the agents on PATH that have none.
func buildSetupView(settings prefs) setupView {
	v := setupView{Version: version.Number, Agents: []agentState{}, Questions: decisions(settings), Pricing: buildPricingSetup(settings)}
	installs := readInstalls()
	for _, e := range installs {
		state := "connected"
		if e.Registration < registrationVersion {
			state = "older"
		}
		v.Agents = append(v.Agents, agentState{Name: e.Agent, Scope: installScope(e), State: state})
	}
	for _, name := range foundAgents() {
		if slices.ContainsFunc(installs, func(entry install) bool { return entry.Agent == name }) {
			continue
		}
		state := "not connected"
		if slices.Contains(settings.Declined, name) {
			state = "declined"
		}
		v.Agents = append(v.Agents, agentState{Name: name, State: state})
	}
	return v
}

// experimentPrefix starts the id of a question about an experimental feature.
const experimentPrefix = "experimental:"

// experiments are the opt-in features setup can turn on. Each is named as
// experimentalOn asks for it; the experimental commands in help.go are listed
// here too (a test keeps both in step).
var experiments = []struct{ name, title, text, example, how, impact string }{
	{"continuity", "Resume snapshot after compaction",
		"Keeps claude oriented after it compacts its conversation or resumes a session.",
		"gascoigne: your state before the conversation was compacted or resumed, from the transcript:\n" +
			"- last request: add a settings page\n" +
			"- open tasks:\n  - [in_progress] write tests\n" +
			"- files edited, newest first: /p/a.go, /p/b.go\n" +
			"- recent failed commands: `go test ./...`",
		"A session-start hook reads the session's transcript when claude compacts or resumes, and adds a summary to the session context. It uses only the agent's own calls (never tool output) and cuts secrets from command lines.",
		"Adds up to about 500 tokens at each compaction or resume, so the agent keeps its place instead of rediscovering it. Not yet measured against the rework it saves."},
	{"compaction", "Compact claude's conversation earlier",
		"Makes claude compact its conversation at 250k tokens of context instead of near the model's limit.",
		"$ gascoigne context\n27 compactions, from 553.3k to 198.9k of context: cost $25, saved $183 of cache reads, net $158\n\n" +
			"~/.claude/settings.json:  \"autoCompactWindow\": 250000",
		"Writes claude's autoCompactWindow setting (user settings, where gascoigne is installed for every project) and removes it again when you turn this off. A value you set yourself is never touched. Claude only; other agents have no such setting here.",
		"Every request re-reads the whole conversation, so long sessions cost most of their money at large contexts. Compacting earlier cuts that, but a summary forgets detail the full conversation had, and each compaction writes the new context to the cache once. On one real machine compacting paid off ($158 net over 30 days); yours may differ, see gascoigne context. Takes effect in claude sessions started afterwards."},
	{"learn", "Learn from your sessions",
		"Adds the learn command: it proposes instruction lines from habits that cost context.",
		"$ gascoigne learn\nhabit  command   sessions  runs\nrerun  go test   3         7\n\n" +
			"lines for CLAUDE.local.md (-apply writes them):\n" +
			"- `go test` output arrives shortened with a hint: read the rest with\n  `gascoigne raw <id> -grep PATTERN` instead of running it again.",
		"Reads your recent runs in the ledger and finds commands run again for output that was already shortened, and large output nothing could shorten, seen in at least two sessions. -apply writes the lines to CLAUDE.local.md as a marked block; -remove takes it out.",
		"Nothing changes in your sessions until you apply lines. Then agents see a few extra instruction lines (about 30 tokens each) meant to prevent repeated reads. Its value on real ledgers is not measured yet."},
	{"prompts", "Instruction text report",
		"Adds the prompts command: what your agents are told, and what it costs.",
		"$ gascoigne prompts\nclaude sends 7.5k tokens of instructions with every request\n" +
			"source             tokens  via\nCLAUDE.md             2.7k  project\n" +
			".harness/rules.md      978  @import from CLAUDE.md\n\n" +
			"findings\n  large-import  docs/notes.md  2.5k  sent with every request",
		"Finds the instruction files an agent loads (CLAUDE.md and its imports, memory, skill and agent descriptions, AGENTS.md), counts and prices them, and flags large imports and duplicates. With -terse it can drop articles and fillers from instruction files in place (a .bak is kept).",
		"Listing changes nothing. The point is to show instruction text that you pay for on every request; -terse edits your files, so review the result."},
	{"prose", "Prose compression with a local model",
		"Downloads a small language model (274 MB) that drops low-value words from long prose, and turns the prose optimizer on.",
		"$ gascoigne prose install\ngascoigne prose: onnxruntime-linux-x64-1.29.0.tgz\n  sha256 c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba\n  downloaded, checksum matches\n\n" +
			"setup runs this download when you turn it on; the background server keeps the model loaded\n" +
			"a README paragraph: 'It sits between an agent and what the agent reads, shrinks it' -> 'sits between agent and what agent reads, shrinks it'",
		"prose install downloads the model (274 MB) and the ONNX Runtime libraries, checking their hashes. With this on, plain paragraphs of text and markdown go through the model on your GPU (CUDA or CoreML) or CPU; numbers, paths, flags, identifiers, negations, code and links are kept. The original stays retrievable with gascoigne raw.",
		"Lossy: about 8-10% fewer tokens on prose-heavy text, none on code, tables or command output. Needs the downloads and a few hundred MB of memory while it runs (in the background server, which keeps the model loaded)."},
}

// experimentDecisions are one on/off question per experimental feature. They
// always have an answer (off until turned on), so they never nag.
func experimentDecisions(settings prefs) []decision {
	var ds []decision
	for _, x := range experiments {
		current := string(toggleOff)
		if slices.Contains(settings.Experimental, x.name) {
			current = string(toggleOn)
		}
		ds = append(ds, decision{ID: experimentPrefix + x.name, Group: "experimental", Current: current, Default: string(toggleOff), Optional: true,
			Title: x.title, Text: x.text, Example: x.example, How: x.how, Impact: x.impact,
			Options: []option{{string(toggleOn), "Turn on", ""}, {string(toggleOff), "Leave off", "The feature stays unavailable."}}})
	}
	return ds
}

// proseModelID is the question that picks the prose model.
const proseModelID = "prose_model"

// proseChoices are the models the prose question offers, the default first;
// a test keeps them in step with prose.Names.
var proseChoices = []struct{ name, size, detail string }{
	{prose.Kompress, "274 MB", "Trained on general prose: drops articles and fillers, and keeps the meaning of the fixtures at the default threshold."},
	{prose.Ctxprune, "563 MB", "Trained on tool output, logs, code and web text: about 3x faster on the CPU, keeps fewer words on prose."},
}

// proseSize is the download size of a prose model, for messages.
func proseSize(name string) string {
	for _, c := range proseChoices {
		if c.name == name {
			return c.size
		}
	}
	return "?"
}

// proseModelDecision picks the model the prose experiment uses. It always has
// an answer, like the experiments: only a prose that is on downloads it.
func proseModelDecision(settings prefs) decision {
	d := decision{ID: proseModelID, Group: "experimental", Optional: true, Default: prose.Kompress,
		Current: cmp.Or(settings.ProseModel, prose.Kompress),
		Title:   "Model for prose compression",
		Text:    "Which model the prose experiment runs. With prose on, setup downloads the one you pick and checks its files."}
	for _, c := range proseChoices {
		d.Options = append(d.Options, option{c.name, c.name + " (" + c.size + ")", c.detail})
	}
	return d
}

// integratePrefix starts the id of a question about an agent found on PATH.
const integratePrefix = "integrate:"

// newAgents are agents on PATH that have no recorded install and were not
// declined; the setup page asks about each. gascoigne only signals them.
func newAgents(settings prefs) []string {
	var out []string
	installs := readInstalls()
	for _, name := range foundAgents() {
		if !slices.ContainsFunc(installs, func(entry install) bool { return entry.Agent == name }) {
			out = append(out, name)
		}
	}
	return out
}

// declinedAs is the saved answer for an agent question: "no" once declined.
func declinedAs(settings prefs, name string) string {
	if slices.Contains(settings.Declined, name) {
		return "no"
	}
	return ""
}

// mcpModeDecision is the standing choice between gascoigne shortening results
// by itself and the agent calling its tools. It always has an answer (auto), so
// it never nags.
func mcpModeDecision(settings prefs) decision {
	return decision{ID: "mcp_mode", Group: "permissions", Optional: true, Default: mcpAuto,
		Current: cmp.Or(settings.MCPMode, mcpAuto),
		Title:   "How gascoigne reaches your agent",
		Text:    "Automatic routes tool results through gascoigne's hooks. Manual leaves your agent's tools alone.",
		Options: []option{
			{mcpAuto, "Automatic", "Hooks shorten command and MCP output after it runs, and claude's Grep, Glob and WebFetch are denied so searches and pages come through gascoigne."},
			{mcpManual, "Manual", "Nothing is rewritten or denied. The agent calls gascoigne's tools (fetch, compress, retrieve, show, stats) itself when it wants a result shortened."},
		}}
}

// detectedSentence says what auto finds, for the billing question.
func detectedSentence() string {
	var parts []string
	for _, d := range detectedAgents() {
		parts = append(parts, d.Agent+": "+d.Mode+" ("+d.Why+")")
	}
	return "Found here: " + strings.Join(parts, "; ") + "."
}

// billingDecision is the standing choice between showing dollar figures and not. It always has
// an answer (api), so it never nags; prices, files and commands are set with gascoigne pricing.
func billingDecision(settings prefs) decision {
	return decision{ID: "billing", Group: "costs", Optional: true, Default: cost.BillingAuto,
		Current: cmp.Or(settings.Pricing.Billing, cost.BillingAuto),
		Title:   "How you pay for the model",
		Text:    "Dollar figures only mean something when you pay per token. " + detectedSentence(),
		Options: []option{
			{cost.BillingAuto, "Find out for me", "Each agent is looked up by how it is signed in here: an API key or a cloud provider means per token, an account sign-in means a plan, and a plan shows no dollars."},
			{cost.BillingAPI, "Per token (API) for everything", "Costs and gain show what the saved tokens are worth at list prices, or at the prices you set below."},
			{cost.BillingSubscription, "A flat plan for everything", "No dollar figures anywhere: a subscription bills nothing per token, so only tokens saved are shown."},
		}}
}

func pendingDecisions(settings prefs) []decision {
	return slices.DeleteFunc(decisions(settings), func(choice decision) bool { return choice.Current != "" })
}

// answerState is what applying answers builds up: the prefs to save, the
// report lines so far, and whether the installed configs need writing again.
type answerState struct {
	prefs   *prefs
	out     []string
	rewrite bool
}

// answerHandlers say what each answer does, by the start of the question id.
// An id that is only the prefix (allow, release) has an empty name.
var answerHandlers = []struct {
	prefix string
	apply  func(state *answerState, name, value string)
}{
	{integratePrefix, func(state *answerState, name, value string) {
		state.prefs.Declined = slices.DeleteFunc(state.prefs.Declined, func(existing string) bool { return existing == name })
		if value == "no" {
			state.prefs.Declined = append(state.prefs.Declined, name)
			return
		}
		state.out = append(state.out, integrate(name))
		state.rewrite = true
	}},
	{experimentPrefix, func(state *answerState, name, value string) {
		state.prefs.Experimental = slices.DeleteFunc(state.prefs.Experimental, func(existing string) bool { return existing == name })
		if toggle(value) == toggleOn {
			state.prefs.Experimental = append(state.prefs.Experimental, name)
		}
		state.rewrite = state.rewrite || name == "compaction" // reaches claude's settings
	}},
	{proseModelID, func(state *answerState, _, value string) { state.prefs.ProseModel = value }},
	{"release", func(state *answerState, _, value string) { state.prefs.ReleaseCheck = toggle(value) }},
	{"mcp_review", func(state *answerState, _, value string) { state.prefs.MCPReview = toggle(value) }},
	{"update", func(state *answerState, _, value string) {
		if value == "no" {
			state.prefs.UpdateDeclined = registrationVersion
			return
		}
		state.rewrite = true
	}},
	// A changed choice of these reaches the installed configs.
	{"allow", func(state *answerState, _, value string) { state.prefs.Allow, state.rewrite = value, true }},
	{"autosync", func(state *answerState, _, value string) { state.prefs.Autosync, state.rewrite = toggle(value), true }},
	{"mcp_mode", func(state *answerState, _, value string) { state.prefs.MCPMode, state.rewrite = value, true }},
	{"billing", func(state *answerState, _, value string) { state.prefs.Pricing.Billing = value }},
}

// checkAnswers refuses an answer for an unknown question or with a value the
// question does not offer.
func checkAnswers(settings prefs, answers map[string]string) error {
	questions := decisions(settings)
	for id, value := range answers {
		index := slices.IndexFunc(questions, func(choice decision) bool { return choice.ID == id })
		if index < 0 {
			return fmt.Errorf("unknown question %q", id)
		}
		if !slices.ContainsFunc(questions[index].Options, func(candidate option) bool { return candidate.Value == value }) {
			return fmt.Errorf("%s: %q is not an option", id, value)
		}
	}
	return nil
}

// applyAnswers saves the answers (decision id → option value) and writes the
// user-scope installs again so the new choices reach the configs. Unknown ids
// or values refuse the whole request.
func applyAnswers(answers map[string]string) ([]string, error) {
	settings := readPrefs()
	if err := checkAnswers(settings, answers); err != nil {
		return nil, err
	}
	state := answerState{prefs: &settings}
	for id, value := range answers {
		for _, handler := range answerHandlers {
			if name, ok := strings.CutPrefix(id, handler.prefix); ok {
				handler.apply(&state, name, value)
				break
			}
		}
	}
	slices.Sort(settings.Declined)
	slices.Sort(settings.Experimental)
	if err := settings.save(); err != nil {
		return nil, err
	}
	if !state.rewrite {
		return state.out, nil
	}
	return append(state.out, rewriteInstalls()...), nil
}

// needsProseModel reports a prose experiment that is on, by this setup's
// answer or already in the config, while its files are missing or fail their
// checksums: setup then fetches them, so the optimizer works right away.
func needsProseModel(answers map[string]string) bool {
	on := cmp.Or(answers[experimentPrefix+"prose"], boolWord(experimentalOn("prose")))
	if toggle(on) != toggleOn {
		return false
	}
	dir, err := prose.ModelDir()
	if err != nil {
		return false
	}
	err = prose.Verify(dir, configProseModel())
	if err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne prose:", err)
	}
	return err != nil
}

func boolWord(on bool) string {
	if on {
		return string(toggleOn)
	}
	return ""
}

// installProseModel downloads the chosen prose model and the ONNX Runtime;
// progress and checksums go to stderr.
func installProseModel(ctx context.Context) error {
	dir, err := prose.ModelDir()
	if err != nil {
		return err
	}
	return prose.Install(ctx, dir, "auto", configProseModel())
}

// integrate sets gascoigne up for an agent the user said yes to: the same
// user-level install as gascoigne install -g <agent>.
func integrate(name string) string {
	a, ok := agents[name]
	exe, err := os.Executable()
	if !ok || err != nil {
		return name + ": could not be set up, run gascoigne install -g " + name
	}
	cmd := installedName(exe)
	if _, err := a.add(context.Background(), cmd, true); err != nil {
		return fmt.Sprintf("%s: could not be set up (%v), run gascoigne install -g %s", name, err, name)
	}
	if err := recordInstall(name, true, cmd, false); err != nil {
		return name + ": set up, but not recorded: " + err.Error()
	}
	return name + ": set up"
}

// rewriteInstalls writes every user-scope install again, and says which
// project installs need gascoigne install run in their project.
func rewriteInstalls() []string {
	unlock, ok := lockInstalls()
	if !ok {
		return []string{"another gascoigne is updating the installs; run gascoigne upgrade later"}
	}
	defer unlock()
	var out []string
	list := readInstalls()
	for i, e := range list {
		a, known := agents[e.Agent]
		switch {
		case !known:
		case !e.Global:
			out = append(out, fmt.Sprintf("%s in %s: run gascoigne install there to apply", e.Agent, e.Dir))
		case a.resync(e) != nil:
			out = append(out, e.Agent+": could not be updated, run gascoigne install -g")
		default:
			list[i].Registration = registrationVersion
			out = append(out, e.Agent+": updated")
		}
	}
	if err := writeInstalls(list); err != nil {
		out = append(out, "could not record the update: "+err.Error())
	}
	return out
}

// experiment is one switched-on experimental feature as the dashboard lists it.
type experiment struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// enabledExperiments lists the experimental features that are on. The prose
// model says whether it can run; the others have no state beyond being on.
func enabledExperiments() []experiment {
	out := []experiment{}
	for _, e := range experiments {
		if !experimentalOn(e.name) {
			continue
		}
		x := experiment{Name: e.name, Title: e.title, State: string(toggleOn), Detail: e.text}
		if e.name == "prose" {
			st := loadProseStatus()
			x.State, x.Detail = st.State, st.Detail
		}
		out = append(out, x)
	}
	return out
}
