package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// periodLabel says a number of hours the way a person would: "24 hours", "14 days".
func periodLabel(hours int) string {
	if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d days", hours/24)
}

// printContext writes the report as text, the terminal form of the page.
func printContext(writer io.Writer, report contextReport) {
	if len(report.Sessions) == 0 {
		fmt.Fprintf(writer, "No Claude Code transcripts with usage in the last %s (looked in %s).\n", periodLabel(report.Hours), claudeProjects())
		return
	}
	if report.Billing == cost.BillingSubscription {
		printContextFlat(writer, report)
		return
	}
	fmt.Fprintf(writer, "%d sessions, %d requests, about %s at API list prices, last %s\n", len(report.Sessions), report.Turns, dollars(report.CostUSD), periodLabel(report.Hours))
	if report.SubagentTurns > 0 {
		fmt.Fprintf(writer, "plus subagents: %d requests, about %s\n", report.SubagentTurns, dollars(report.SubagentUSD))
	}
	fmt.Fprintln(writer)
	printBucketsCost(writer, report.Buckets)
	printRewritesCost(writer, report.Rewrites)
	printCacheTTL(writer, report.CacheTTL)
	printInjectedCost(writer, report.Injected)
	if c := report.Compactions; c.Count > 0 {
		fmt.Fprintf(writer, "\n%d compactions, from %s to %s of context: cost %s, saved %s of cache reads, net %s\n",
			c.Count, human(c.MeanBefore), human(c.MeanAfter), dollars(c.CostUSD), dollars(c.SavedUSD), dollars(c.NetUSD))
	}
	printWhatIfs(writer, report)
	printLargestByCost(writer, report.Sessions)
	for _, assumption := range report.Assumptions {
		fmt.Fprintln(writer, "\n"+assumption)
	}
}

// newTable is the aligned table every section of the text report draws.
func newTable(writer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
}

func printBucketsCost(writer io.Writer, buckets []contextBucket) {
	table := newTable(writer)
	fmt.Fprintln(table, "CONTEXT OF A REQUEST\tREQUESTS\tCOST\tSHARE")
	for _, bucket := range buckets {
		fmt.Fprintf(table, "%s\t%d\t%s\t%.0f%%\n", bucket.Label, bucket.Turns, dollars(bucket.CostUSD), 100*bucket.Share)
	}
	table.Flush()
}

func printRewritesCost(writer io.Writer, rewrites []cacheRewrite) {
	fmt.Fprintln(writer, "\nContext written to the cache again instead of read:")
	table := newTable(writer)
	fmt.Fprintln(table, "CAUSE\tREQUESTS\tTOKENS\tEXTRA COST\tSHARE")
	for _, rewrite := range rewrites {
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\t%.1f%%\n", rewrite.Cause, rewrite.Turns, human(rewrite.Tokens), dollars(rewrite.CostUSD), 100*rewrite.Share)
	}
	table.Flush()
}

func printInjectedCost(writer io.Writer, injected []injectedSource) {
	if len(injected) == 0 {
		return
	}
	fmt.Fprintln(writer, "\nText added to conversations besides your prompts (every later request re-reads it):")
	table := newTable(writer)
	fmt.Fprintln(table, "SOURCE\tTIMES\tPER TIME\tCOST\tSHARE")
	for _, source := range injected[:min(len(injected), 8)] {
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\t%.1f%%\n", source.Source, source.Times, human(source.PerTime), dollars(source.CostUSD), 100*source.Share)
	}
	table.Flush()
}

func printWhatIfs(writer io.Writer, report contextReport) {
	fmt.Fprintln(writer, "\nIf conversations had been compacted when they reached:")
	table := newTable(writer)
	fmt.Fprintln(table, "CONTEXT\tSESSIONS AFFECTED\tCOMPACTIONS\tSAVED")
	for _, option := range report.WhatIf {
		fmt.Fprintf(table, "%dk\t%d\t%d\t%s (%.0f%%)\n", option.Threshold/1000, option.Sessions, option.Compactions, dollars(option.SavedUSD), option.SavedPct)
	}
	table.Flush()
	if report.BestWindow > 0 {
		fmt.Fprintf(writer, "\nThe best of these what-ifs is %dk. The experimental switch in gascoigne setup -experimental compacts claude at %dk.\n", report.BestWindow/1000, compactWindow/1000)
	}
}

func printLargestByCost(writer io.Writer, sessions []sessionCost) {
	fmt.Fprintln(writer, "\nLargest sessions:")
	top := slices.Clone(sessions)
	slices.SortFunc(top, func(left, right sessionCost) int { return cmp.Compare(right.CostUSD, left.CostUSD) })
	table := newTable(writer)
	fmt.Fprintln(table, "PROJECT\tSTART\tREQUESTS\tPEAK\tCOMPACTIONS\tCOST")
	for _, session := range top[:min(len(top), 8)] {
		fmt.Fprintf(table, "%s\t%s\t%d\t%s\t%d\t%s\n", cmp.Or(session.Project, "-"), session.Start.Local().Format("2006-01-02"), session.Turns, human(session.Peak), session.Compactions, dollars(session.CostUSD))
	}
	table.Flush()
}

// printContextFlat is printContext for a flat plan: how big conversations get, in requests and
// tokens, with no dollars.
func printContextFlat(writer io.Writer, report contextReport) {
	fmt.Fprintf(writer, "%d sessions, %d requests, last %s (flat plan: no dollar figures)\n\n", len(report.Sessions), report.Turns, periodLabel(report.Hours))
	printBucketsFlat(writer, report)
	printRewritesFlat(writer, report.Rewrites)
	printInjectedFlat(writer, report.Injected)
	if c := report.Compactions; c.Count > 0 {
		fmt.Fprintf(writer, "\n%d compactions, from %s to %s of context\n", c.Count, human(c.MeanBefore), human(c.MeanAfter))
	}
	printLargestByPeak(writer, report.Sessions)
}

func printBucketsFlat(writer io.Writer, report contextReport) {
	table := newTable(writer)
	fmt.Fprintln(table, "CONTEXT OF A REQUEST\tREQUESTS\tSHARE OF REQUESTS")
	for _, bucket := range report.Buckets {
		share := 0.0
		if report.Turns > 0 {
			share = 100 * float64(bucket.Turns) / float64(report.Turns)
		}
		fmt.Fprintf(table, "%s\t%d\t%.0f%%\n", bucket.Label, bucket.Turns, share)
	}
	table.Flush()
}

func printRewritesFlat(writer io.Writer, rewrites []cacheRewrite) {
	fmt.Fprintln(writer, "\nContext written to the cache again instead of read:")
	table := newTable(writer)
	fmt.Fprintln(table, "CAUSE\tREQUESTS\tTOKENS")
	for _, rewrite := range rewrites {
		fmt.Fprintf(table, "%s\t%d\t%s\n", rewrite.Cause, rewrite.Turns, human(rewrite.Tokens))
	}
	table.Flush()
}

func printInjectedFlat(writer io.Writer, injected []injectedSource) {
	if len(injected) == 0 {
		return
	}
	fmt.Fprintln(writer, "\nText added to conversations besides your prompts (every later request re-reads it):")
	table := newTable(writer)
	fmt.Fprintln(table, "SOURCE\tTIMES\tPER TIME")
	for _, source := range injected[:min(len(injected), 8)] {
		fmt.Fprintf(table, "%s\t%d\t%s\n", source.Source, source.Times, human(source.PerTime))
	}
	table.Flush()
}

func printLargestByPeak(writer io.Writer, sessions []sessionCost) {
	fmt.Fprintln(writer, "\nLargest sessions:")
	top := slices.Clone(sessions)
	slices.SortFunc(top, func(left, right sessionCost) int { return cmp.Compare(right.Peak, left.Peak) })
	table := newTable(writer)
	fmt.Fprintln(table, "PROJECT\tSTART\tREQUESTS\tPEAK\tCOMPACTIONS")
	for _, session := range top[:min(len(top), 8)] {
		fmt.Fprintf(table, "%s\t%s\t%d\t%s\t%d\n", cmp.Or(session.Project, "-"), session.Start.Local().Format("2006-01-02"), session.Turns, human(session.Peak), session.Compactions)
	}
	table.Flush()
}

// printCacheTTL says whether the 1-hour cache would have paid, with the figures.
func printCacheTTL(writer io.Writer, ttl cacheTTL) {
	switch {
	case ttl.Turns < ttlMinTurns:
		fmt.Fprintf(writer, "\n1-hour cache: too few requests (%d, from %d) to tell.\n", ttl.Turns, ttlMinTurns)
	case ttl.Worth:
		fmt.Fprintf(writer, "\n1-hour cache would have paid: reading the %s tokens rewritten after pauses of 5 to 60 minutes saves %s, its dearer writes cost %s more (net %s).\n",
			human(ttl.PauseTokens), dollars(ttl.SavedUSD), dollars(ttl.ExtraUSD), dollars(ttl.SavedUSD-ttl.ExtraUSD))
	default:
		fmt.Fprintf(writer, "\n1-hour cache would not have paid: it saves %s on pauses of 5 to 60 minutes, its dearer writes cost %s more.\n",
			dollars(ttl.SavedUSD), dollars(ttl.ExtraUSD))
	}
}
