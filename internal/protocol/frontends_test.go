package protocol

import (
	"reflect"
	"testing"
)

func roundTrip(t *testing.T, msg any) any {
	t.Helper()
	data, err := Encode(msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := Decode(data[:len(data)-1])
	if err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return decoded
}

// A frontend says on subscribing what it is and what it understands, so a
// driver can ask the running process rather than the binary on disk.
func TestSubscribeCarriesTheFrontend(t *testing.T) {
	in := &SubscribeMsg{Type: TypeSubscribe, Passive: true,
		Frontend: &FrontendInfo{Pid: 42, Version: "v1.2.3", Features: []string{"doc"}}}
	out, ok := roundTrip(t, in).(*SubscribeMsg)
	if !ok || !reflect.DeepEqual(out.Frontend, in.Frontend) {
		t.Errorf("frontend after a round trip: %+v", out)
	}
	// An agent's subscribe (no frontend) stays as it was on the wire.
	data, _ := Encode(&SubscribeMsg{Type: TypeSubscribe, Events: []string{"x"}})
	if string(data) != `{"type":"subscribe","events":["x"]}`+"\n" {
		t.Errorf("a subscribe without a frontend changed shape: %s", data)
	}
}

func TestGetFrontendsRoundTrip(t *testing.T) {
	if _, ok := roundTrip(t, &GetFrontendsMsg{Type: TypeGetFrontends}).(*GetFrontendsMsg); !ok {
		t.Error("get_frontends did not decode to *GetFrontendsMsg")
	}
	in := &GetFrontendsResponse{Type: TypeGetFrontendsResponse, Features: []string{"doc"},
		Frontends: []FrontendInfo{{Pid: 7, Version: "dev", Features: []string{"doc"}}}}
	out, ok := roundTrip(t, in).(*GetFrontendsResponse)
	if !ok || !reflect.DeepEqual(out, in) {
		t.Errorf("get_frontends_response after a round trip: %+v", out)
	}
}

func TestDocPaneRoundTrip(t *testing.T) {
	if out, ok := roundTrip(t, &DocPaneMsg{Type: TypeDocPane, State: "off"}).(*DocPaneMsg); !ok || out.State != "off" {
		t.Errorf("doc_pane after a round trip: %+v", out)
	}
	in := &DocPaneResponse{Type: TypeDocPaneResponse, Success: true, Message: "m", Delivered: 1}
	if out, ok := roundTrip(t, in).(*DocPaneResponse); !ok || !reflect.DeepEqual(out, in) {
		t.Errorf("doc_pane_response after a round trip: %+v", out)
	}
}

func TestCommentsRoundTrip(t *testing.T) {
	if out, ok := roundTrip(t, &CommentsMsg{Type: TypeComments, State: "off"}).(*CommentsMsg); !ok || out.State != "off" {
		t.Errorf("comments after a round trip: %+v", out)
	}
	in := &CommentsResponse{Type: TypeCommentsResponse, Success: true, Message: "m", Delivered: 1}
	if out, ok := roundTrip(t, in).(*CommentsResponse); !ok || !reflect.DeepEqual(out, in) {
		t.Errorf("comments_response after a round trip: %+v", out)
	}
}
