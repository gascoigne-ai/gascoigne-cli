package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Check judges one piece of output (a stream or a file's content). Scenario authors never
// write one: they pick a constructor below, which keeps a scenario row readable.
type Check func(box *sandbox, got string) error

// Has needs every part to appear in the output. ${home}, ${work} and ${bin} expand in parts.
func Has(parts ...string) Check {
	return func(box *sandbox, got string) error {
		for _, part := range parts {
			part = box.expand(part)
			if !strings.Contains(got, part) {
				return fmt.Errorf("want %q in output", part)
			}
		}
		return nil
	}
}

// Lacks needs every part to be absent from the output.
func Lacks(parts ...string) Check {
	return func(box *sandbox, got string) error {
		for _, part := range parts {
			part = box.expand(part)
			if strings.Contains(got, part) {
				return fmt.Errorf("want no %q in output", part)
			}
		}
		return nil
	}
}

// Is needs the output to equal want exactly.
func Is(want string) Check {
	return func(_ *sandbox, got string) error {
		if got != want {
			return fmt.Errorf("output %q, want %q", got, want)
		}
		return nil
	}
}

// Empty needs no output at all.
func Empty() Check { return Is("") }

// Like needs the output to match a regular expression.
func Like(pattern string) Check {
	return func(_ *sandbox, got string) error {
		matched, err := regexp.MatchString(pattern, got)
		if err != nil {
			return fmt.Errorf("pattern %q: %w", pattern, err)
		}
		if !matched {
			return fmt.Errorf("output does not match %q", pattern)
		}
		return nil
	}
}

// SameAsFile needs the output to be byte-exact the file in the working dir.
func SameAsFile(path string) Check {
	return func(box *sandbox, got string) error {
		want, err := os.ReadFile(filepath.Join(box.vars["work"], path))
		if err != nil {
			return err
		}
		if got != string(want) {
			return fmt.Errorf("output (%d bytes) differs from %s (%d bytes)", len(got), path, len(want))
		}
		return nil
	}
}

// ShorterThan needs the output to be smaller than the file in the working dir.
func ShorterThan(path string) Check {
	return func(box *sandbox, got string) error {
		original, err := os.ReadFile(filepath.Join(box.vars["work"], path))
		if err != nil {
			return err
		}
		if len(got) >= len(original) {
			return fmt.Errorf("output is %d bytes, want fewer than the %d of %s", len(got), len(original), path)
		}
		return nil
	}
}

// JSONField needs the output to be JSON with want at a dotted path, e.g. "total.runs".
func JSONField(path string, want any) Check {
	return func(_ *sandbox, got string) error {
		var node any
		if err := json.Unmarshal([]byte(got), &node); err != nil {
			return fmt.Errorf("output is not JSON: %w", err)
		}
		for key := range strings.SplitSeq(path, ".") {
			object, ok := node.(map[string]any)
			if !ok {
				return fmt.Errorf("json path %s: %q is not inside an object", path, key)
			}
			if node, ok = object[key]; !ok {
				return fmt.Errorf("json path %s: no key %q", path, key)
			}
		}
		if fmt.Sprint(node) != fmt.Sprint(want) {
			return fmt.Errorf("json %s = %v, want %v", path, node, want)
		}
		return nil
	}
}
