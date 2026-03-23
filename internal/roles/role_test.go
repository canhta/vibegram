package roles

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/state"
)

type mockCaller struct {
	response string
	err      error
	calls    int
}

func (m *mockCaller) Call(ctx context.Context, prompt string) (string, error) {
	m.calls++
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func makeSnap() state.Snapshot {
	return state.Snapshot{
		SessionID:         "sess-1",
		Phase:             "editing",
		Status:            "running",
		ReplyAttemptCount: 1,
	}
}

func makeEvent(et events.EventType, summary string) events.NormalizedEvent {
	return events.NormalizedEvent{
		EventType: et,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

func TestRoleExecutorReturnsReplyDecision(t *testing.T) {
	caller := &mockCaller{response: `{"action":"reply","message":"Try running go mod tidy"}`}
	e := NewExecutor(caller)
	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeBlocked, "missing dep"))
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Action != ActionReply {
		t.Errorf("expected ActionReply, got %v", d.Action)
	}
	if d.Message != "Try running go mod tidy" {
		t.Errorf("expected message 'Try running go mod tidy', got %q", d.Message)
	}
}

func TestRoleExecutorReturnsEscalateDecision(t *testing.T) {
	caller := &mockCaller{response: `{"action":"escalate","reason":"risky operation"}`}
	e := NewExecutor(caller)
	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeBlocked, "delete all"))
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Action != ActionEscalate {
		t.Errorf("expected ActionEscalate, got %v", d.Action)
	}
	if d.Reason != "risky operation" {
		t.Errorf("expected reason 'risky operation', got %q", d.Reason)
	}
}

func TestRoleExecutorReturnsNoopDecision(t *testing.T) {
	caller := &mockCaller{response: `{"action":"noop"}`}
	e := NewExecutor(caller)
	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeToolActivity, "ran tool"))
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Action != ActionNoop {
		t.Errorf("expected ActionNoop, got %v", d.Action)
	}
}

func TestRoleExecutorMalformedResponseFailsClosed(t *testing.T) {
	caller := &mockCaller{response: `not json at all`}
	e := NewExecutor(caller)
	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeBlocked, "err"))
	if err != nil {
		t.Fatalf("Decide should not return error on malformed response, got %v", err)
	}
	if d.Action != ActionNoop {
		t.Errorf("expected ActionNoop on malformed response, got %v", d.Action)
	}
}

func TestRoleExecutorEmptyMessageFailsClosed(t *testing.T) {
	caller := &mockCaller{response: `{"action":"reply","message":""}`}
	e := NewExecutor(caller)
	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeQuestion, "what?"))
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Action != ActionNoop {
		t.Errorf("expected ActionNoop for empty reply message, got %v", d.Action)
	}
}

func TestRoleExecutorContextCancelled(t *testing.T) {
	caller := &mockCaller{err: context.Canceled}
	e := NewExecutor(caller)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := e.Decide(ctx, makeSnap(), makeEvent(events.EventTypeBlocked, "err"))
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if d.Action != ActionNoop {
		t.Errorf("expected ActionNoop on cancelled context, got %v", d.Action)
	}
}

func TestRoleExecutorUsesStrongCallerForLongQuestion(t *testing.T) {
	cheap := &mockCaller{response: `{"action":"reply","message":"cheap"}`}
	strong := &mockCaller{response: `{"action":"reply","message":"strong"}`}
	e := NewExecutor(cheap, strong)

	d, err := e.Decide(context.Background(), makeSnap(), makeEvent(events.EventTypeQuestion, strings.Repeat("long architecture question ", 20)))
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Action != ActionReply || d.Message != "strong" {
		t.Fatalf("decision = %+v, want strong reply", d)
	}
	if cheap.calls != 0 {
		t.Fatalf("cheap caller calls = %d, want 0", cheap.calls)
	}
	if strong.calls != 1 {
		t.Fatalf("strong caller calls = %d, want 1", strong.calls)
	}
}
