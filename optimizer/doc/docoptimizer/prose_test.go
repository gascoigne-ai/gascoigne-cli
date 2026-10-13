package docoptimizer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// TestMain doubles as the helper: tests start the test binary itself with
// FAKE_PROSE set. The fake drops every word starting with "drop".
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_PROSE") {
	case "":
		os.Exit(m.Run())
	case "ok":
		fmt.Println("ready")
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var req request
			json.Unmarshal(sc.Bytes(), &req)
			var res response
			for _, w := range req.Words {
				res.Keep = append(res.Keep, !strings.HasPrefix(w, "drop"))
			}
			json.NewEncoder(os.Stdout).Encode(res)
		}
	case "short":
		fmt.Println("ready")
		bufio.NewScanner(os.Stdin).Scan()
		fmt.Println(`{"keep":[true]}`)
	case "stderr":
		fmt.Fprintln(os.Stderr, "onnx exploded")
		os.Exit(1)
	case "crash":
		os.Exit(1)
	}
}

func proseOptimizer(t *testing.T, mode string) *Optimizer {
	t.Setenv("FAKE_PROSE", mode)
	exe, _ := os.Executable()
	cfg := &Config{MaxCodeLines: 6, MaxTokens: 4000, Model: Model{Enabled: true, Command: []string{exe}, Threshold: 0.5, MinWords: 4}}
	return &Optimizer{cfg: cfg, helper: &helper{cfg: &cfg.Model, log: slog.New(slog.DiscardHandler)}}
}

func TestProse(t *testing.T) {
	in := strings.Join([]string{
		"# Title with drop words and more words here",
		"keep one dropb keep three dropd",
		"keep five 42 and `code` drops",
		"",
		"```",
		"drop inside code fence stays as is here",
		"```",
		"",
		"drop a b",
		"- drop list item stays as is as well",
	}, "\n")
	want := strings.Join([]string{
		"# Title with drop words and more words here",
		"keep one keep three",
		"keep five 42 and `code`",
		"",
		"```",
		"drop inside code fence stays as is here",
		"```",
		"",
		"drop a b",
		"- drop list item stays as is as well",
	}, "\n")
	o := proseOptimizer(t, "ok")
	defer o.Shutdown(t.Context())
	if got := strings.Join(o.prose(strings.Split(in, "\n")), "\n"); got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestOptimizeText(t *testing.T) {
	body := "keep one dropb keep three dropd"
	tests := []struct{ name, mode, want string }{
		{"compressed", "ok", "keep one keep three"},
		{"wrong flag count", "short", body},
		{"helper crashes", "crash", body},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := proseOptimizer(t, tt.mode)
			defer o.Shutdown(t.Context())
			p := &payload.Payload{Blocks: []payload.Block{{Type: "doc/text", Text: body}}}
			if err := o.Optimize(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(p.Blocks[0].Text); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTabularAndMarkersUntouched(t *testing.T) {
	table := []string{"drop1  drop2  drop3", "drop4  drop5  drop6", "drop7  drop8  drop9", "dropa  dropb  dropc"}
	prose := "keep one dropx keep three dropy"
	marker := []string{"… 12 lines omitted", "[gascoigne: optimized 10→5 tokens]"}
	o := proseOptimizer(t, "ok")
	defer o.Shutdown(t.Context())
	in := append(append(append([]string{}, table...), "", prose, ""), marker...)
	want := append(append(append([]string{}, table...), "", "keep one keep three", ""), marker...)
	if got := o.prose(in); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestHelperFailureCarriesStderr(t *testing.T) {
	o := proseOptimizer(t, "stderr")
	_, err := o.helper.keep([]string{"a"})
	if err == nil || !strings.Contains(err.Error(), "onnx exploded") {
		t.Fatalf("got %v", err)
	}
}

func TestStartWarmsHelper(t *testing.T) {
	for _, warm := range []bool{true, false} {
		o := proseOptimizer(t, "ok")
		o.cfg.Model.Warm = warm
		if err := o.Start(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for ; warm && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			o.helper.mu.Lock()
			up := o.helper.cmd != nil
			o.helper.mu.Unlock()
			if up {
				break
			}
		}
		o.helper.mu.Lock()
		up := o.helper.cmd != nil
		o.helper.mu.Unlock()
		o.Shutdown(t.Context())
		if up != warm {
			t.Errorf("warm=%v: helper running = %v", warm, up)
		}
	}
}

func TestDefaultCommandIsSelf(t *testing.T) {
	h := &helper{cfg: &Model{}}
	argv, err := h.argv()
	if err != nil || len(argv) != 3 || argv[1] != "prose" || argv[2] != "serve" {
		t.Fatalf("got %v, %v", argv, err)
	}
}

func TestHelperGivenUp(t *testing.T) {
	o := proseOptimizer(t, "crash")
	for range helperMaxFailures {
		o.helper.keep([]string{"a"})
	}
	if _, err := o.helper.keep([]string{"a"}); err != errHelperGone {
		t.Fatalf("got %v, want errHelperGone", err)
	}
}

func TestDisabledByDefault(t *testing.T) {
	o := testOptimizer()
	body := "drop one dropb dropc dropd\n\n\n\nx"
	p := &payload.Payload{Blocks: []payload.Block{{Type: "doc/text", Text: body}}}
	if err := o.Optimize(t.Context(), p); err != nil || p.Blocks[0].Text != body {
		t.Fatalf("text must stay byte-exact: %q, %v", p.Blocks[0].Text, err)
	}
}

func TestModelValidate(t *testing.T) {
	ok := Config{MaxCodeLines: 6, MaxTokens: 500, Model: Model{Enabled: true, Threshold: 0.5, MinWords: 1}}
	bad := ok
	bad.Model.Threshold = 1
	if ok.Validate() != nil || bad.Validate() == nil {
		t.Fatal("threshold must be inside (0, 1)")
	}
}
