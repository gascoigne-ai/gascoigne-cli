// Command paramnames reports one-letter function, method and closure parameter
// names (see .harness/common/rules.md, Naming). It prints counts per package and
// exits 0 whenever the report ran, so it is not a gate.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// skippedDir reports directories the walk never enters.
func skippedDir(name string) bool {
	return name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && name != "." && name != "..")
}

// collect checks every .go file under paths (file, dir or dir/...). Files that
// fail to parse are returned as errors; the rest still count.
func collect(paths []string) ([]Violation, []error) {
	var violations []Violation
	var errs []error
	checkFile := func(path string) {
		src, err := os.ReadFile(path)
		if err == nil {
			var found []Violation
			found, err = check(path, src)
			violations = append(violations, found...)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	for _, path := range paths {
		root := strings.TrimSuffix(path, "...")
		if root == "" {
			root = "."
		}
		err := filepath.WalkDir(filepath.Clean(root), func(current string, entry fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir() && current != filepath.Clean(root) && skippedDir(entry.Name()):
				return filepath.SkipDir
			case !entry.IsDir() && strings.HasSuffix(current, ".go"):
				checkFile(current)
			}
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return violations, errs
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("paramnames", flag.ContinueOnError)
	flags.SetOutput(stderr)
	verbose := flags.Bool("v", false, "print every violation as file:line: name")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: paramnames [-v] <file|dir|dir/...>...")
		return exitUsage
	}
	violations, errs := collect(flags.Args())
	for _, err := range errs {
		fmt.Fprintf(stderr, "paramnames: %v\n", err)
	}
	if *verbose {
		slices.SortFunc(violations, func(left, right Violation) int {
			return cmp.Or(cmp.Compare(left.File, right.File), cmp.Compare(left.Line, right.Line))
		})
		for _, violation := range violations {
			fmt.Fprintf(stdout, "%s:%d: %s\n", violation.File, violation.Line, violation.Name)
		}
	}
	for _, count := range counts(violations) {
		fmt.Fprintf(stdout, "%s\t%d\n", count.Package, count.Count)
	}
	fmt.Fprintf(stdout, "total\t%d\n", len(violations))
	if len(errs) > 0 {
		return exitError
	}
	return exitOK
}
