package main

import (
	"testing"

	"github.com/DevOfPie/Mustur/internal/tmuxtest"
)

// Tests here drive session.Adapters with no Runner, which shell out to real
// tmux; this keeps them off the owner's server (MUS-F-0161).
func TestMain(m *testing.M) { tmuxtest.Main(m) }
