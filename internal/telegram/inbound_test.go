package telegram

import (
	"strings"
	"testing"
)

type mockPTYWriter struct {
	written map[string][]string
}

func (m *mockPTYWriter) Write(sessionID string, text string) error {
	if m.written == nil {
		m.written = make(map[string][]string)
	}
	m.written[sessionID] = append(m.written[sessionID], text)
	return nil
}

type mockSessionLookup struct {
	sessions map[int]string
}

func (m *mockSessionLookup) ByThreadID(threadID int) (string, bool) {
	id, ok := m.sessions[threadID]
	return id, ok
}

func makeInboundRouter() (*InboundRouter, *mockPTYWriter) {
	auth := NewAuthorizer([]int64{1001}, []int64{1002})
	pty := &mockPTYWriter{}
	sessions := &mockSessionLookup{
		sessions: map[int]string{42: "sess-abc"},
	}
	r := &InboundRouter{
		Auth:            auth,
		Sessions:        sessions,
		PTY:             pty,
		GeneralThreadID: 1,
	}
	return r, pty
}

func TestInboundRouterSessionTopicMessageRoutesToPTY(t *testing.T) {
	r, pty := makeInboundRouter()
	reply, err := r.HandleUpdate(1001, 42, "go mod tidy")
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	_ = reply
	if len(pty.written["sess-abc"]) == 0 {
		t.Error("expected message to be written to PTY")
	}
	if pty.written["sess-abc"][0] != "go mod tidy" {
		t.Errorf("expected PTY text 'go mod tidy', got %q", pty.written["sess-abc"][0])
	}
}

func TestInboundRouterUnauthorizedMessageIsRejected(t *testing.T) {
	r, pty := makeInboundRouter()
	reply, err := r.HandleUpdate(9999, 42, "some command")
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	_ = reply
	if len(pty.written["sess-abc"]) > 0 {
		t.Error("expected observer message to be rejected, not written to PTY")
	}
}

func TestInboundRouterUnknownTopicIsIgnored(t *testing.T) {
	r, pty := makeInboundRouter()
	reply, err := r.HandleUpdate(1001, 99, "some text")
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	if reply != "" {
		t.Errorf("expected empty reply for unknown topic, got %q", reply)
	}
	for _, writes := range pty.written {
		if len(writes) > 0 {
			t.Error("expected no PTY writes for unknown topic")
		}
	}
}

func TestInboundRouterGeneralTopicStatusCommand(t *testing.T) {
	r, _ := makeInboundRouter()
	reply, err := r.HandleUpdate(1001, 1, "status")
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	if reply == "" {
		t.Error("expected non-empty reply for status command")
	}
	if !strings.Contains(strings.ToLower(reply), "status") {
		t.Errorf("expected reply to contain 'status', got %q", reply)
	}
}

func TestInboundRouterGeneralTopicUnknownCommand(t *testing.T) {
	r, pty := makeInboundRouter()
	reply, err := r.HandleUpdate(1001, 1, "unknowncommand")
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	_ = reply
	for _, writes := range pty.written {
		if len(writes) > 0 {
			t.Error("expected no PTY writes for unknown general command")
		}
	}
}
