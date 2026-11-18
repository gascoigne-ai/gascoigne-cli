package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

//go:embed setup.html
var setupHTML []byte

// setupHandlers serves the page and its two calls, all behind the token.
func setupHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.Write(setupHTML)
	})
	// The dashboard's Change button: the page needs a token, which only a
	// same-origin POST (localOnly) may mint.
	mux.HandleFunc("POST /api/setup/link", func(response http.ResponseWriter, _ *http.Request) {
		link, err := setupLink()
		respond(response, map[string]string{"url": link}, err)
	})
	mux.HandleFunc("GET /api/setup", func(response http.ResponseWriter, request *http.Request) {
		if !validSetupToken(request.URL.Query().Get("token")) {
			http.Error(response, "this link expired or was used: run gascoigne setup", http.StatusForbidden)
			return
		}
		respond(response, buildSetupView(readPrefs()), nil)
	})
	mux.HandleFunc("POST /api/setup", saveSetup)
}

// saveSetup takes the page's answers (and pricing form), behind the token,
// and says what was done.
func saveSetup(response http.ResponseWriter, request *http.Request) {
	var req struct {
		Token   string            `json:"token"`
		Answers map[string]string `json:"answers"`
		Pricing *pricingForm      `json:"pricing"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}
	if !validSetupToken(req.Token) {
		http.Error(response, "this link expired or was used: run gascoigne setup", http.StatusForbidden)
		return
	}
	// The pricing form is checked before anything is saved: all or nothing.
	saved := readPrefs()
	if req.Pricing != nil {
		if err := req.Pricing.apply(&saved.Pricing); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
	}
	lines, err := applyAnswers(req.Answers)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Pricing != nil {
		// applyAnswers saved the answers (billing among them); the form's interval and prices go on top.
		current := readPrefs()
		current.Pricing.TTL, current.Pricing.Prices = saved.Pricing.TTL, saved.Pricing.Prices
		if err := current.save(); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		applyPricing(false)
		lines = append(lines, "costs: pricing settings saved")
	}
	os.Remove(setupTokenFile()) // one answer per link
	if needsProseModel(req.Answers) {
		// Minutes of download: not within this request.
		go func() {
			if err := installProseModel(context.Background()); err != nil {
				fmt.Fprintln(os.Stderr, "gascoigne: prose download failed, run gascoigne prose install:", err)
			}
		}()
		name := configProseModel()
		lines = append(lines, fmt.Sprintf("prose: downloading %s (%s) in the background; gascoigne prose status says when it is ready", name, proseSize(name)))
	}
	respond(response, map[string][]string{"done": lines}, nil)
}

// cmdSetup asks the open questions: in the browser when one can open here,
// else in the terminal (-t forces the terminal). -y takes the defaults; -all
// asks the answered questions again.
func cmdSetup(ctx context.Context, args []string) error {
	fset := newFlags("setup")
	yes := fset.Bool("y", false, "take the default answer to every question")
	all := fset.Bool("all", false, "also ask the questions already answered")
	term := fset.Bool("t", false, "ask in the terminal even where a browser can open")
	experimental := fset.Bool("experimental", false, "ask only about the experimental features")
	if err := fset.Parse(args); err != nil {
		return err
	}
	p := readPrefs()
	ds := pendingDecisions(p)
	if *all {
		ds = decisions(p)
	}
	if *experimental {
		ds = append(experimentDecisions(p), proseModelDecision(p))
	}
	if len(ds) == 0 {
		fmt.Println("nothing to decide; setup -all asks again")
		return repairProse(ctx, nil)
	}
	if useBrowser(*yes, *term) {
		link, err := setupLink()
		if err == nil {
			openBrowser(link)
			fmt.Printf("answer in the browser: %s\n(gascoigne setup -t asks here instead)\n", link)
			return nil
		}
		fmt.Fprintln(os.Stderr, "gascoigne:", err, "- asking here instead")
	}
	if !*yes && !isTerminal(os.Stdin) {
		return errors.New("setup: needs a browser or a terminal to ask; -y takes the defaults")
	}
	answers, err := ask(os.Stdin, os.Stdout, ds, *yes)
	if err != nil {
		return err
	}
	lines, err := applyAnswers(answers)
	for _, l := range lines {
		fmt.Println(l)
	}
	if err != nil {
		return err
	}
	return repairProse(ctx, answers)
}

// repairProse fetches the prose files again when they are missing or corrupt.
func repairProse(ctx context.Context, answers map[string]string) error {
	if !needsProseModel(answers) {
		return nil
	}
	name := configProseModel()
	fmt.Printf("prose: downloading %s (%s) and ONNX Runtime\n", name, proseSize(name))
	if err := installProseModel(ctx); err != nil {
		return fmt.Errorf("prose is on but the download failed (%w); run gascoigne prose install to retry", err)
	}
	return nil
}

// useBrowser decides where setup asks: the page, unless defaults were asked
// for, the terminal was chosen, or no browser can open here (ssh, CI, server off).
func useBrowser(yes, term bool) bool {
	return !yes && !term && hasDisplay(runtime.GOOS) && os.Getenv("CI") == "" && !serverOff()
}

// ask puts each question to w and reads the number of the chosen option from
// r; an empty line takes the default.
func ask(input io.Reader, out io.Writer, ds []decision, defaults bool) (map[string]string, error) {
	in := bufio.NewScanner(input)
	answers := map[string]string{}
	for _, d := range ds {
		answers[d.ID] = d.Default
		if defaults {
			continue
		}
		fmt.Fprintf(out, "\n%s\n%s\n", d.Title, d.Text)
		for _, part := range []struct{ name, text string }{{"Example", d.Example}, {"How it works", d.How}, {"Impact", d.Impact}} {
			if part.text != "" {
				fmt.Fprintf(out, "  %s:\n    %s\n", part.name, strings.ReplaceAll(part.text, "\n", "\n    "))
			}
		}
		for i, o := range d.Options {
			mark := " "
			if o.Value == d.Default {
				mark = "*"
			}
			fmt.Fprintf(out, " %s %d) %s\n", mark, i+1, o.Label)
			if o.Detail != "" {
				fmt.Fprintf(out, "       %s\n", o.Detail)
			}
		}
		for {
			fmt.Fprint(out, "choose [default *]: ")
			if !in.Scan() {
				return nil, errors.New("setup: no answer")
			}
			line := strings.TrimSpace(in.Text())
			if line == "" {
				break
			}
			var n int
			if _, err := fmt.Sscanf(line, "%d", &n); err == nil && n >= 1 && n <= len(d.Options) {
				answers[d.ID] = d.Options[n-1].Value
				break
			}
			fmt.Fprintf(out, "enter 1-%d\n", len(d.Options))
		}
	}
	return answers, nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
