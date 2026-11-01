package pkglist

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func TestNoiseRulesVerify(t *testing.T) {
	if err := noise.Verify(); err != nil {
		t.Error(err)
	}
}

func TestCompress(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"pip list", "Package  Version\n-------- -------\nflask    3.0.3\nclick    8.1.7\n", "flask 3.0.3\nclick 8.1.7\n"},
		{"pip list outdated keeps columns", "Package Version Latest Type\n------- ------- ------ -----\nflask   3.0.2   3.0.3  wheel\n", "Package Version Latest Type\nflask 3.0.2 3.0.3 wheel\n"},
		{"pip freeze", "Flask==3.0.3\nclick==8.1.7\n-e git+https://x/y.git#egg=y\n", "Flask 3.0.3\nclick 8.1.7\n-e git+https://x/y.git#egg=y\n"},
		{"apt", "Listing... Done\nbash/noble,now 5.2.21 amd64 [installed]\nca-certificates/noble,now 2024 all [installed,automatic]\n", "bash 5.2.21 amd64\nca-certificates 2024 all automatic\n"},
		{"gem", "\n*** LOCAL GEMS ***\n\njson (default: 2.9.1)\nrake (13.0.6, 12.3.3)\n", "json 2.9.1\nrake 13.0.6,12.3.3\n"},
		{"same version shares a line", "bluez 5.87-2\nbluez-libs 5.87-2\nbolt 0.9.11-3\nbtop 1.4.7-1\n", "5.87-2: bluez bluez-libs\nbolt 0.9.11-3\nbtop 1.4.7-1\n"},
		{"go list", "example.com/app\ngolang.org/x/net v0.61.0\nexample.com/old v1.0.0 => ../old\n", "example.com/app\ngolang.org/x/net v0.61.0\nexample.com/old v1.0.0 => ../old\n"},
		{"foreign line breaks a run", "a 1\nb 1\nERROR: broken\nc 1\n", "1: a b\nERROR: broken\nc 1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compress.Join(Compress(compress.Lines(tc.in)))
			if strings.TrimSpace(got) != strings.TrimSpace(tc.want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}
