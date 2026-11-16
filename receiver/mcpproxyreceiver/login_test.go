package mcpproxyreceiver

import "testing"

func TestLoginFlowTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  loginPhase
		event loginEvent
		want  loginPhase
		ok    bool
	}{
		{"first start", loginIdle, loginStart, loginWaiting, true},
		{"retry after failure", loginFailed, loginStart, loginWaiting, true},
		{"success", loginWaiting, loginSucceed, loginDone, true},
		{"failure", loginWaiting, loginFail, loginFailed, true},
		{"no second flow while one waits", loginWaiting, loginStart, loginWaiting, false},
		{"no restart once logged in", loginDone, loginStart, loginDone, false},
		{"cannot finish before start", loginIdle, loginSucceed, loginIdle, false},
		{"failure cannot follow success", loginDone, loginFail, loginDone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := &loginFlow{phase: tt.from}
			if ok := flow.advance(tt.event); ok != tt.ok || flow.phase != tt.want {
				t.Fatalf("advance(%s) from %s = %v, phase %s; want %v, %s", tt.event, tt.from, ok, flow.phase, tt.ok, tt.want)
			}
		})
	}
}
