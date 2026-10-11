package testreport

import (
	"strings"
	"testing"
)

func TestJUnit(t *testing.T) {
	xml := `<?xml version="1.0"?>
<testsuites>
 <testsuite name="api" time="1.5">
  <testcase classname="OrdersTest" name="lists" time="0.1"/>
  <testcase classname="OrdersTest" name="creates" time="0.2">
   <failure message="expected 201 got 500" type="AssertionError">at OrdersTest.creates(OrdersTest.java:40)</failure>
   <system-out>POST /orders 500</system-out>
  </testcase>
  <testcase classname="OrdersTest" name="deletes"><skipped message="flaky"/></testcase>
  <testcase classname="OrdersTest" name="boots"><error message="NullPointerException"/></testcase>
 </testsuite>
</testsuites>`
	want := `api: 1 passed, 2 failed, 1 skipped (1.5s)
FAIL OrdersTest::creates: expected 201 got 500
  at OrdersTest.creates(OrdersTest.java:40)
  POST /orders 500
SKIP OrdersTest::deletes flaky
ERROR OrdersTest::boots: NullPointerException`
	got, err := JUnit(xml)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != want {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	single, err := JUnit(`<testsuite name="one"><testcase name="a"/></testsuite>`)
	if err != nil || strings.Join(single, "\n") != "one: 1 passed, 0 failed, 0 skipped" {
		t.Fatalf("single suite: %q %v", single, err)
	}
}

func TestCoverage(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "lcov",
			in:   "TN:\nSF:a.ts\nDA:1,3\nDA:2,0\nDA:3,0\nDA:4,1\nDA:5,0\nLF:5\nLH:2\nend_of_record\nSF:b.ts\nDA:1,1\nLF:1\nLH:1\nend_of_record",
			want: "total: 50.0% of 2 files\na.ts 40.0% (2/5 lines) uncovered: 2-3,5\n1 files at or above 80.0%",
		},
		{
			name: "go cover -func",
			in:   "x/a.go:3:\tNew\t\t100.0%\nx/a.go:9:\tparse\t\t20.0%\nx/b.go:4:\tRun\t\t90.0%\ntotal:\t\t(statements)\t70.0%",
			want: "total: 70.0% of 2 files\nx/a.go 60.0%: parse 20.0%\n1 files at or above 80.0%",
		},
		{
			name: "not a report",
			in:   "hello\nworld",
			want: "hello\nworld",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(Coverage(strings.Split(tt.in, "\n"), 80), "\n"); got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
