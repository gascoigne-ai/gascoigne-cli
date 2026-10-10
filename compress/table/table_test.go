package table

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
)

func render(t *testing.T, in string, max int) string {
	t.Helper()
	tb, ok := Parse(compress.Lines(in))
	if !ok {
		t.Fatalf("no table in:\n%s", in)
	}
	return compress.Join(Render(tb, Options{MaxRows: max, MaxCell: 30, Rows: rows.Options{HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2}}))
}

func TestPsql(t *testing.T) {
	in := " id | name  | status\n----+-------+--------\n  1 | alice | active\n  2 | bob   | active\n  3 | carol | banned\n(3 rows)\n"
	got := render(t, in, 30)
	for _, w := range []string{"id | name | status", "3 | carol | banned", "(3 rows)"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestMySQLBox(t *testing.T) {
	in := "+----+-------+\n| id | name  |\n+----+-------+\n|  1 | alice |\n|  2 | bob   |\n+----+-------+\n2 rows in set (0.00 sec)\n"
	got := render(t, in, 30)
	if !strings.Contains(got, "id | name\n1 | alice\n2 | bob\n2 rows in set") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestAlignedHoistAndSample(t *testing.T) {
	var b strings.Builder
	b.WriteString("NAME      READY   STATUS             RESTARTS   NAMESPACE\n")
	for i := range 40 {
		status, ready := "Running", "1/1"
		if i == 23 {
			status, ready = "CrashLoopBackOff", "0/1"
		}
		b.WriteString("pod-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "    " + ready + "     " + status + strings.Repeat(" ", 19-len(status)) + "0          default\n")
	}
	got := render(t, b.String(), 8)
	for _, w := range []string{"same in all rows: RESTARTS=0, NAMESPACE=default", "CrashLoopBackOff", "items omitted"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestFixedWidthEmptyCells(t *testing.T) {
	in := "CONTAINER ID   IMAGE   PORTS                  NAMES\nabc            web     0.0.0.0:80->80/tcp     web-1\ndef            db                             db-1\nghi            cache                          cache-1\n"
	got := render(t, in, 30)
	if !strings.Contains(got, "def | db |  | db-1") {
		t.Fatalf("got:\n%s", got)
	}
}
