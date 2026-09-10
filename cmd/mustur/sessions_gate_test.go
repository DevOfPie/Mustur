package main

import (
	"testing"

	"github.com/DevOfPie/Mustur/internal/session"
)

// Three states, because "the default set" and "no gate at all" are different
// answers and an empty flag has to mean one of them. MUS-D-0153 promises the
// second and the tree had no way to say it for one commit.
func TestTheGateFlagCanSayNoGateAtAll(t *testing.T) {
	if got := gateFlag(""); got != nil {
		t.Errorf("unset gave %v, want nil so the adapter uses its default", got)
	}
	none := gateFlag("none")
	if none == nil || len(none) != 0 {
		t.Errorf("none gave %v, want an empty set that is not nil", none)
	}
	if session.Gated(none, "default", "Bash") {
		t.Error("a session that gates nothing held a call anyway")
	}
	if got := gateFlag("Bash, Edit"); len(got) != 2 || got[0] != "Bash" || got[1] != "Edit" {
		t.Errorf("a list gave %v", got)
	}
}
