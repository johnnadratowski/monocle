package client

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/testenv"
)

// Every test in this package runs against a throwaway config dir, data dir and
// database: its tests drive real engines, which write wherever those point.
func TestMain(m *testing.M) { testenv.Main("client", m.Run) }
