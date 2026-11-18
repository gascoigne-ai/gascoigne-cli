package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// costTable prices the savings per model: once at first read, and with the
// rereads of every later request while tool results stay in context.
func (p painter) costTable(writer io.Writer, costs mcptool.Costs) {
	if len(costs.Savings) == 0 && len(costs.Unpriced) == 0 {
		return
	}
	p.section(writer, "cost · what the saved tokens are worth")
	var rows [][]cell
	assumed := false
	for _, s := range costs.Savings {
		name := s.Model
		if s.Assumed {
			name += "*"
			assumed = true
		}
		rows = append(rows, []cell{
			{text: name},
			{text: s.Tokenizer, style: dim},
			{text: human(s.ModelTokens), right: true},
			{text: dollars(s.FirstRead), right: true},
			{text: dollars(s.InContext), right: true, style: green},
		})
	}
	if len(costs.Savings) > 1 {
		rows = append(rows, []cell{{text: "total", style: bold}, {}, {}, {text: dollars(costs.FirstRead), right: true, style: bold},
			{text: dollars(costs.InContext), right: true, style: bold + ";" + green}})
	}
	p.table(writer, []string{"model", "tokenizer", "saved tok", "first read", "in context"}, rows)
	fmt.Fprintln(writer, p.paint(dim, fmt.Sprintf(" in context: each tool result is read again by ~%.0f later requests (cache reads) until compaction · -rereads N", costs.Rereads)))
	if assumed {
		fmt.Fprintln(writer, p.paint(dim, " * no model recorded: the agent's default, set GASCOIGNE_MODEL or -model to record it"))
	}
	if len(costs.Unpriced) > 0 {
		fmt.Fprintln(writer, p.paint(yellow, " not priced: "+strings.Join(costs.Unpriced, ", ")))
	}
}

// usageTable shows what the proxied API requests were billed, as the
// providers reported it, priced at list prices.
func (p painter) usageTable(writer io.Writer, usage []ledger.UsageStat) {
	if len(usage) == 0 {
		return
	}
	// A plan bills nothing per token: its rows show no price, and the column goes when no row has one.
	dollarsShown := slices.ContainsFunc(usage, func(stat ledger.UsageStat) bool {
		return cost.BillingOf(stat.Agent).Mode != cost.BillingSubscription
	})
	p.section(writer, "api usage · measured by the LLM proxy")
	var rows [][]cell
	var total float64
	unpriced := false
	for _, u := range usage {
		price := cell{text: "?", right: true, style: dim}
		if cost.BillingOf(u.Agent).Mode == cost.BillingSubscription {
			price = cell{text: "plan", right: true, style: dim}
		} else if m, ok := cost.Lookup(u.Model); ok {
			usd := (float64(u.Input)*m.Input + float64(u.CacheRead)*m.CacheRead +
				float64(u.CacheWrite-u.CacheWrite1h)*m.CacheWrite + float64(u.CacheWrite1h)*oneHourWrite*m.Input + float64(u.Output)*m.Output) / 1e6
			price, total = cell{text: dollars(usd), right: true}, total+usd
		} else {
			unpriced = true
		}
		rows = append(rows, []cell{
			{text: cmp.Or(u.Agent, "-")}, {text: cmp.Or(u.Model, "-")},
			{text: strconv.Itoa(u.Requests), right: true},
			{text: human(u.MeanContext), right: true}, {text: human(u.PeakContext), right: true},
			{text: human(u.Input), right: true}, {text: human(u.CacheRead), right: true},
			{text: human(u.CacheWrite), right: true}, {text: human(u.Output), right: true}, price,
		})
	}
	if len(usage) > 1 {
		rows = append(rows, []cell{{text: "total", style: bold}, {}, {}, {}, {}, {}, {}, {}, {}, {text: dollars(total), right: true, style: bold}})
	}
	head := []string{"agent", "model", "requests", "context", "peak", "input", "cache read", "cache write", "output", "cost"}
	if !dollarsShown { // a flat plan bills nothing per token
		head = head[:len(head)-1]
		for i := range rows {
			rows[i] = rows[i][:len(head)]
		}
		unpriced = false
	}
	p.table(writer, head, rows)
	fmt.Fprintln(writer, p.paint(dim, " context: tokens one request carries (input, cache read and write), mean and peak · cache writes at the 5-minute or 1-hour price as reported, where the provider reports the split"))
	if unpriced {
		fmt.Fprintln(writer, p.paint(dim, " ? no known price for the model"))
	}
}

// usageDaysTable shows how many API requests a day the proxy saw and how big
// the conversation each one re-read was: the numbers that drive the bill.
func (p painter) usageDaysTable(writer io.Writer, days []ledger.UsageDay) {
	if len(days) < 2 {
		return
	}
	p.section(writer, "api requests by day · measured by the LLM proxy")
	var rows [][]cell
	for _, d := range days {
		rows = append(rows, []cell{
			{text: d.Day}, {text: strconv.Itoa(d.Requests), right: true},
			{text: human(d.MeanContext), right: true}, {text: human(d.PeakContext), right: true},
			{text: human(d.CacheRead), right: true}, {text: human(d.CacheWrite), right: true},
		})
	}
	p.table(writer, []string{"day", "requests", "context", "peak", "cache read", "cache write"}, rows)
}

func dollars(amount float64) string {
	if amount < 10 {
		return fmt.Sprintf("$%.2f", amount)
	}
	return fmt.Sprintf("$%.0f", amount)
}
