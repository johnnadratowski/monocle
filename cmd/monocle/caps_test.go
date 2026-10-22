package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/tui"
)

// The TUI reports itself to the engine as this process, this build, and the
// TUI's features.
func TestFrontendInfoIsThisTUI(t *testing.T) {
	info := frontendInfo()
	if info.Pid != os.Getpid() || info.Version != version || !reflect.DeepEqual(info.Features, tui.Features) {
		t.Errorf("frontendInfo() = %+v", info)
	}
}

// Without --json, a line per frontend and then what all of them understand.
func TestCapsText(t *testing.T) {
	r := &protocol.GetFrontendsResponse{Features: []string{"doc"},
		Frontends: []protocol.FrontendInfo{{Pid: 7, Version: "v2", Features: []string{"doc"}}, {Pid: 8, Version: "v2", Features: []string{"doc", "zoom"}}}}
	want := "tui pid 7 (v2): doc\ntui pid 8 (v2): doc zoom\nall: doc\n"
	if got := capsText(r); got != want {
		t.Errorf("capsText =\n%s\nwant\n%s", got, want)
	}
	if got := capsText(&protocol.GetFrontendsResponse{Features: []string{}}); got != "no Monocle TUI attached\n" {
		t.Errorf("with none attached: %q", got)
	}
}
