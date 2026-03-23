package policy

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
)

type mockExecutor struct {
	decision roles.Decision
	called   bool
}

func (m *mockExecutor) Decide(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (roles.Decision, error) {
	m.called = true
	return m.decision, nil
}

func makeSnap(replyCount int) state.Snapshot {
	return state.Snapshot{
		SessionID:         "sess-1",
		Phase:             "editing",
		Status:            "running",
		ReplyAttemptCount: replyCount,
	}
}

func makeEvent(et events.EventType) events.NormalizedEvent {
	return events.NormalizedEvent{
		EventType: et,
		Summary:   "test event",
		Timestamp: time.Now(),
	}
}

func TestPolicyQuestionWithSafeAutoReply(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "Use testify"}}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeQuestion))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionReply {
		t.Errorf("expected ActionReply, got %v", d.Action)
	}
	if !exec.called {
		t.Error("expected executor to be called for question event")
	}
}

func TestPolicyBlockedWithLowAttemptCount(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "try this"}}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(1), makeEvent(events.EventTypeBlocked))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionReply {
		t.Errorf("expected ActionReply, got %v", d.Action)
	}
	if !exec.called {
		t.Error("expected executor to be called for blocked event with low attempt count")
	}
}

func TestPolicyBlockedWithHighAttemptCount(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "try this"}}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(3), makeEvent(events.EventTypeBlocked))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionEscalate {
		t.Errorf("expected ActionEscalate when attempt count >= 3, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called when attempt count >= 3")
	}
}

func TestPolicyApprovalNeededAlwaysEscalates(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "ok"}}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeApprovalNeeded))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionEscalate {
		t.Errorf("expected ActionEscalate for approval_needed, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for approval_needed")
	}
}

func TestPolicyFailedAlwaysEscalates(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "ok"}}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeFailed))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionEscalate {
		t.Errorf("expected ActionEscalate for failed, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for failed")
	}
}

func TestPolicyToolActivityIsNoop(t *testing.T) {
	exec := &mockExecutor{}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeToolActivity))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionNoop {
		t.Errorf("expected ActionNoop for tool_activity, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for tool_activity")
	}
}

func TestPolicyPhaseChangedIsNoop(t *testing.T) {
	exec := &mockExecutor{}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypePhaseChanged))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionNoop {
		t.Errorf("expected ActionNoop for phase_changed, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for phase_changed")
	}
}

func TestPolicyDoneIsNoop(t *testing.T) {
	exec := &mockExecutor{}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeDone))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionNoop {
		t.Errorf("expected ActionNoop for done, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for done")
	}
}

func TestPolicyBlockerResolvedIsNoop(t *testing.T) {
	exec := &mockExecutor{}
	engine := NewEngine(exec)
	d, err := engine.Evaluate(context.Background(), makeSnap(0), makeEvent(events.EventTypeBlockerResolved))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionNoop {
		t.Errorf("expected ActionNoop for blocker_resolved, got %v", d.Action)
	}
	if exec.called {
		t.Error("expected executor NOT to be called for blocker_resolved")
	}
}

func TestPolicyQuestionEscalatesWhenAutoReplyBudgetIsExhausted(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "try this"}}
	engine := NewEngine(exec)
	snap := makeSnap(2)

	d, err := engine.Evaluate(context.Background(), snap, makeEvent(events.EventTypeQuestion))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionEscalate {
		t.Fatalf("expected ActionEscalate when reply budget is exhausted, got %v", d.Action)
	}
	if exec.called {
		t.Fatal("expected executor NOT to be called after budget is exhausted")
	}
}

func TestPolicyQuestionSkipsRepeatedSummary(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "use testify"}}
	engine := NewEngine(exec)
	snap := makeSnap(1)
	snap.LastQuestion = "which test package should I use?"
	snap.RecentEvents = []string{
		"which test package should I use?",
		"which test package should I use?",
	}

	event := makeEvent(events.EventTypeQuestion)
	event.Summary = "which test package should I use?"
	d, err := engine.Evaluate(context.Background(), snap, event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionNoop {
		t.Fatalf("expected ActionNoop for repeated question, got %v", d.Action)
	}
	if exec.called {
		t.Fatal("expected executor NOT to be called for repeated question")
	}
}

func TestPolicyRiskyBlockedSummaryEscalatesWithoutExecutor(t *testing.T) {
	exec := &mockExecutor{decision: roles.Decision{Action: roles.ActionReply, Message: "go ahead"}}
	engine := NewEngine(exec)
	event := makeEvent(events.EventTypeBlocked)
	event.Summary = "Should I use the production API token on the VPS?"

	d, err := engine.Evaluate(context.Background(), makeSnap(0), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Action != roles.ActionEscalate {
		t.Fatalf("expected ActionEscalate for risky summary, got %v", d.Action)
	}
	if exec.called {
		t.Fatal("expected executor NOT to be called for risky summary")
	}
}
