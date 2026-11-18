package mcptool

import "testing"

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"":                    "''",
		"/usr/bin/gascoigne":  "/usr/bin/gascoigne",
		"a-b_c.d:e@f+g%h,i=j": "a-b_c.d:e@f+g%h,i=j",
		"/opt/my tools/g":     "'/opt/my tools/g'",
		"/it's":               `'/it'\''s'`,
		"a!b":                 "'a!b'",
		"~/x":                 "'~/x'",
		"a\nb":                "'a\nb'",
		"$HOME":               "'$HOME'",
	} {
		if got := ShellQuote(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
