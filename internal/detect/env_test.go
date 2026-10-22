package detect

import (
	"slices"
	"testing"
)

func TestSplitEnv(t *testing.T) {
	tests := []struct {
		name, in, env, cmd string
	}{
		{"none", "go test", "", "go test"},
		{"plain", "A=1 go test", "A=1 ", "go test"},
		{"double quoted", `PG="a b" pbbb "hjj"`, `PG="a b" `, `pbbb "hjj"`},
		{"single quoted", `PG='a  b' x`, `PG='a  b' `, "x"},
		{"escaped space", `PG=a\ b x`, `PG=a\ b `, "x"},
		{"escaped quote", `PG="a \"q\" b" x`, `PG="a \"q\" b" `, "x"},
		{"empty value", "PG= x", "PG= ", "x"},
		{"several", `A=1 B="2 2" x`, `A=1 B="2 2" `, "x"},
		{"env only", `A="1 1"`, `A="1 1" `, ""},
		{"unterminated quote", `A="1 x`, "", `A="1 x`},
		{"flag with equals", "--format=x y", "", "--format=x y"},
		{"arg with equals", "x A=1", "", "x A=1"},
		{"leading space", "  A=1 x", "A=1 ", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if env, cmd := SplitEnv(tt.in); env != tt.env || cmd != tt.cmd {
				t.Fatalf("got %q %q, want %q %q", env, cmd, tt.env, tt.cmd)
			}
		})
	}
}

func TestArgvSkipsQuotedEnv(t *testing.T) {
	for in, want := range map[string][]string{
		`PG="a b" pbbb "hjj"`:     {"pbbb", `"hjj"`},
		`sudo PG='a b' make all`:  {"make", "all"},
		`env A="x y" B=2 git log`: {"git", "log"},
		`A=1 command ls`:          {"ls"},
	} {
		if got := Argv(in); !slices.Equal(got, want) {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}
