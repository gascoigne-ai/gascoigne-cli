package pcomb

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	rs := Rules{
		{Name: "cargo progress", Match: Indented(Word("Compiling", "Checking")), Yes: []string{"   Compiling foo"}, No: []string{"Compiling foo"}},
		{Name: "blank", Match: Re(`^\s*$`), Yes: []string{"", "  "}, No: []string{"x"}},
	}
	if err := rs.Verify(); err != nil {
		t.Fatal(err)
	}
	if !rs.Match("   Checking bar") || !rs.Match("") || rs.Match("error: boom") {
		t.Error("Match gave the wrong answer")
	}
	r, ok := rs.First("  ")
	if !ok || r.Name != "blank" {
		t.Errorf("First = %q, %v; want blank", r.Name, ok)
	}
	if _, ok := rs.First("error"); ok {
		t.Error("First should report no rule for an unmatched line")
	}
}

func TestEmptyRulesMatchNothing(t *testing.T) {
	if (Rules{}).Match("") || (Rules(nil)).Match("x") {
		t.Error("an empty table must match nothing")
	}
}

func TestVerifyReportsEveryProblem(t *testing.T) {
	rs := Rules{
		{Name: "a", Match: Prefix("a"), Yes: []string{"b"}, No: []string{"a1"}},
		{Name: "a", Match: Prefix("a"), Yes: []string{"a"}},
		{Name: "no examples", Match: Prefix("c")},
		{Name: "", Match: Prefix("d"), Yes: []string{"d"}},
		{Name: "no matcher", Yes: []string{"e"}},
	}
	err := rs.Verify()
	if err == nil {
		t.Fatal("Verify should fail")
	}
	for _, want := range []string{
		`a: should match "b"`,
		`a: should not match "a1"`,
		"a: duplicate rule name",
		"no examples: needs at least one Yes example",
		"rule needs a Name and a Match",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}
