package main

import (
	"cmp"
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// gainSorts are the orders -sort accepts for grouped tables.
var gainSorts = []string{"saved", "runs", "in", "pct", "name"}

// gainViewFlags are the flags that control how much gain prints.
type gainViewFlags struct {
	show, hide, sort string
	brief            bool
	top, minTokens   int
}

func (f *gainViewFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.show, "show", "", "print only these overview `sections`, comma separated: "+strings.Join(overviewSectionNames(), ", "))
	fs.StringVar(&f.hide, "hide", "", "leave these overview `sections` out (same names as -show)")
	fs.BoolVar(&f.brief, "brief", false, "overview: only outcomes and the trend")
	fs.IntVar(&f.top, "top", 10, "rows per table, the rest is counted (`n`; 0 shows all)")
	fs.StringVar(&f.sort, "sort", "saved", "order of grouped tables: "+strings.Join(gainSorts, ", "))
	fs.IntVar(&f.minTokens, "min-tokens", 0, "hide groups with fewer input tokens (`n`)")
}

// gainView is what the terminal output shows: which overview sections, how
// many rows of each table, in which order.
type gainView struct {
	sections  map[string]bool // nil: all
	top       int
	sort      string
	minTokens int
}

func overviewSectionNames() []string {
	names := make([]string, len(overviewSections))
	for i, s := range overviewSections {
		names[i] = s.name
	}
	return names
}

func (f gainViewFlags) build(overview bool) (gainView, error) {
	if !slices.Contains(gainSorts, f.sort) {
		return gainView{}, fmt.Errorf("gain: unknown -sort %q (%s)", f.sort, strings.Join(gainSorts, ", "))
	}
	if f.top < 0 || f.minTokens < 0 {
		return gainView{}, fmt.Errorf("gain: -top and -min-tokens must not be negative")
	}
	v := gainView{top: f.top, sort: f.sort, minTokens: f.minTokens}
	if f.show == "" && f.hide == "" && !f.brief {
		return v, nil
	}
	if !overview {
		return gainView{}, fmt.Errorf("gain: -show, -hide and -brief shape the overview, not -by")
	}
	v.sections = map[string]bool{}
	for _, n := range overviewSectionNames() {
		v.sections[n] = true
	}
	if f.brief {
		for n := range v.sections {
			v.sections[n] = n == "outcome" || n == "trend"
		}
	}
	only, err := v.sectionList(f.show)
	if err != nil {
		return gainView{}, err
	}
	if len(only) > 0 {
		for n := range v.sections {
			v.sections[n] = false
		}
	}
	for _, n := range only {
		v.sections[n] = true
	}
	hide, err := v.sectionList(f.hide)
	if err != nil {
		return gainView{}, err
	}
	for _, n := range hide {
		v.sections[n] = false
	}
	return v, nil
}

// sectionList splits a comma separated list of section names.
func (v gainView) sectionList(s string) ([]string, error) {
	var out []string
	for n := range strings.SplitSeq(s, ",") {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		if _, ok := v.sections[n]; !ok {
			return nil, fmt.Errorf("gain: unknown section %q (%s)", n, strings.Join(overviewSectionNames(), ", "))
		}
		out = append(out, n)
	}
	return out, nil
}

func (v gainView) show(section string) bool { return v.sections == nil || v.sections[section] }

// rows orders and limits a table's groups; more counts the ones left out.
func (v gainView) rows(rows []mcptool.StatsJSON) (shown []mcptool.StatsJSON, more int) {
	shown = slices.Clone(rows)
	if v.minTokens > 0 {
		shown = slices.DeleteFunc(shown, func(r mcptool.StatsJSON) bool { return r.RawTokens < v.minTokens })
	}
	slices.SortStableFunc(shown, func(a, b mcptool.StatsJSON) int {
		switch v.sort {
		case "runs":
			return cmp.Compare(b.Runs, a.Runs)
		case "in":
			return cmp.Compare(b.RawTokens, a.RawTokens)
		case "pct":
			return cmp.Compare(b.SavedPct, a.SavedPct)
		case "name":
			return cmp.Compare(a.Group, b.Group)
		}
		return cmp.Compare(b.Saved, a.Saved)
	})
	if v.top > 0 && len(shown) > v.top {
		more = len(shown) - v.top
		shown = shown[:v.top]
	}
	return shown, more
}
