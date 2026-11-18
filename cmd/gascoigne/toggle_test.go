package main

import "testing"

func TestEnvOff(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{{"", false}, {"off", true}, {"on", false}, {"0", false}}
	for _, tt := range tests {
		t.Setenv("GASCOIGNE_TEST_TOGGLE", tt.value)
		if got := envOff("GASCOIGNE_TEST_TOGGLE"); got != tt.want {
			t.Errorf("envOff with %q = %v, want %v", tt.value, got, tt.want)
		}
	}
}
