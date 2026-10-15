package trace

import (
	"strings"
	"testing"
)

func TestCycles(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "one-line recursion",
			in:   "Exception in thread \"main\" java.lang.StackOverflowError\n" + strings.Repeat("\tat Tree.depth(Tree.java:12)\n", 6) + "\tat Main.main(Main.java:3)",
			want: "Exception in thread \"main\" java.lang.StackOverflowError\n\tat Tree.depth(Tree.java:12)\n… 1 line above repeated 5 more times\n\tat Main.main(Main.java:3)",
		},
		{
			name: "two-line python frames",
			in: "Traceback (most recent call last):\n" + strings.Repeat("  File \"walk.py\", line 4, in walk\n    return walk(n + 1)\n", 4) +
				"RecursionError: maximum recursion depth exceeded",
			want: "Traceback (most recent call last):\n  File \"walk.py\", line 4, in walk\n    return walk(n + 1)\n… 2 lines above repeated 3 more times\nRecursionError: maximum recursion depth exceeded",
		},
		{
			name: "mutual recursion cycle",
			in:   "top\n" + strings.Repeat("a()\nb()\nc()\n", 3) + "end",
			want: "top\na()\nb()\nc()\n… 3 lines above repeated 2 more times\nend",
		},
		{
			name: "two repeats stay",
			in:   "x\nx\ny",
			want: "x\nx\ny",
		},
		{
			name: "blank lines never cycle",
			in:   "a\n\n\n\n\nb",
			want: "a\n\n\n\n\nb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(Cycles(strings.Split(tt.in, "\n"), 8, 3), "\n")
			if got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestGoroutines(t *testing.T) {
	waiter := func(id, mins string, arg string) string {
		return "goroutine " + id + " [chan receive, " + mins + " minutes]:\n" +
			"example.com/shop/queue.(*Worker).loop(" + arg + ")\n" +
			"\t/src/shop/queue/worker.go:88 +0x9c\n" +
			"created by example.com/shop/queue.Start in goroutine 1\n" +
			"\t/src/shop/queue/worker.go:40 +0x1f\n"
	}
	in := "panic: test timed out after 10m0s\n\n" +
		"goroutine 7 [running]:\nexample.com/shop.TestSync(0xc000102000)\n\t/src/shop/sync_test.go:12 +0x44\n\n" +
		waiter("21", "9", "0xc000180000") + "\n" +
		waiter("22", "9", "0xc000180100") + "\n" +
		waiter("23", "8", "0xc000180200") + "\n" +
		"goroutine 30 [select]:\nexample.com/shop/http.serve()\n\t/src/shop/http/serve.go:20 +0x10\n"
	want := "panic: test timed out after 10m0s\n\n" +
		"goroutine 7 [running]:\nexample.com/shop.TestSync(0xc000102000)\n\t/src/shop/sync_test.go:12 +0x44\n\n" +
		"goroutine 21 [chan receive, 9 minutes]: +2 more on this stack: 22 23\n" +
		"example.com/shop/queue.(*Worker).loop(0xc000180000)\n\t/src/shop/queue/worker.go:88 +0x9c\n" +
		"created by example.com/shop/queue.Start in goroutine 1\n\t/src/shop/queue/worker.go:40 +0x1f\n\n" +
		"goroutine 30 [select]:\nexample.com/shop/http.serve()\n\t/src/shop/http/serve.go:20 +0x10"
	got := strings.Join(Goroutines(strings.Split(strings.TrimSuffix(in, "\n"), "\n"), 3), "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
