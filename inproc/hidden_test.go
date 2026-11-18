package inproc

import "testing"

func TestHiddenKey(t *testing.T) {
	for key, want := range map[string]bool{
		"prompt:ab12": true,
		"prompt:":     true,
		"":            false,
		"prompt":      false,
		"cmd:prompt:": false,
		"git status":  false,
	} {
		if got := HiddenKey(key); got != want {
			t.Errorf("HiddenKey(%q) = %v, want %v", key, got, want)
		}
	}
}
