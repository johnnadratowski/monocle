package main

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/testenv"
)

// Tests here spawn `monocle serve`, which inherits the environment: without
// this it reads the real config and would store media in the real data dir.
func TestMain(m *testing.M) { testenv.Main("cmd", m.Run) }
