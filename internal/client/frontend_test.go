package client

import (
	"reflect"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// A frontend's client says what it is in every subscribe, the first and each
// one after a redial, so a restarted engine learns it again.
func TestFrontendClientSubscribesWithItsInfo(t *testing.T) {
	info := protocol.FrontendInfo{Pid: 9, Version: "v3", Features: []string{"doc"}}
	c := &EngineClient{frontend: &info}
	sub := c.subscribeMsg()
	if !sub.Passive || sub.Frontend == nil || !reflect.DeepEqual(*sub.Frontend, info) {
		t.Errorf("subscribe = %+v, want passive with the frontend", sub)
	}
	if plain := (&EngineClient{}).subscribeMsg(); plain.Frontend != nil {
		t.Errorf("a client given no frontend reported one: %+v", plain.Frontend)
	}
}
