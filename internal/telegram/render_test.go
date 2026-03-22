package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
)

func ev(et events.EventType, summary string) events.NormalizedEvent {
	return events.NormalizedEvent{
		EventType: et,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

func TestRenderSessionStarted(t *testing.T) {
	out := Render(ev(events.EventTypeSessionStarted, "session started"))
	if !strings.Contains(strings.ToLower(out), "started") {
		t.Errorf("expected 'started' in output, got %q", out)
	}
}

func TestRenderBlocked(t *testing.T) {
	out := Render(ev(events.EventTypeBlocked, "missing import path"))
	if !strings.Contains(out, "Blocked:") {
		t.Errorf("expected 'Blocked:' in output, got %q", out)
	}
	if !strings.Contains(out, "missing import path") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderQuestion(t *testing.T) {
	out := Render(ev(events.EventTypeQuestion, "which test framework?"))
	if !strings.Contains(out, "Question:") {
		t.Errorf("expected 'Question:' in output, got %q", out)
	}
	if !strings.Contains(out, "which test framework?") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderDone(t *testing.T) {
	out := Render(ev(events.EventTypeDone, "task complete"))
	if !strings.Contains(out, "Done") {
		t.Errorf("expected 'Done' in output, got %q", out)
	}
	if !strings.Contains(out, "task complete") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderFailed(t *testing.T) {
	out := Render(ev(events.EventTypeFailed, "build error"))
	if !strings.Contains(out, "Failed") {
		t.Errorf("expected 'Failed' in output, got %q", out)
	}
	if !strings.Contains(out, "build error") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderToolActivity(t *testing.T) {
	out := Render(ev(events.EventTypeToolActivity, "ran go build"))
	if !strings.Contains(out, "ran go build") {
		t.Errorf("expected tool summary in output, got %q", out)
	}
}

func TestRenderPhaseChanged(t *testing.T) {
	out := Render(ev(events.EventTypePhaseChanged, "editing"))
	if !strings.Contains(out, "editing") {
		t.Errorf("expected phase in output, got %q", out)
	}
}

func TestRenderFilesChanged(t *testing.T) {
	out := Render(ev(events.EventTypeFilesChanged, "main.go updated"))
	if !strings.Contains(out, "Files:") {
		t.Errorf("expected 'Files:' in output, got %q", out)
	}
	if !strings.Contains(out, "main.go updated") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderTestsChanged(t *testing.T) {
	out := Render(ev(events.EventTypeTestsChanged, "2 tests passed"))
	if !strings.Contains(out, "Tests:") {
		t.Errorf("expected 'Tests:' in output, got %q", out)
	}
	if !strings.Contains(out, "2 tests passed") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderApprovalNeeded(t *testing.T) {
	out := Render(ev(events.EventTypeApprovalNeeded, "delete production db"))
	if !strings.Contains(out, "Approval needed:") {
		t.Errorf("expected 'Approval needed:' in output, got %q", out)
	}
	if !strings.Contains(out, "delete production db") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestRenderLongSummaryTruncated(t *testing.T) {
	longSummary := strings.Repeat("x", 250)
	out := Render(ev(events.EventTypeToolActivity, longSummary))
	if len(out) > 220 {
		t.Errorf("expected output <= 220 chars, got %d", len(out))
	}
}

func TestRenderLongQuestionKeepsFullSummary(t *testing.T) {
	longSummary := strings.Repeat("q", 250) + "?"
	out := Render(ev(events.EventTypeQuestion, longSummary))
	if !strings.Contains(out, longSummary) {
		t.Fatalf("expected full question summary in output, got %q", out)
	}
}

func TestRenderMessageLimit(t *testing.T) {
	longSummary := strings.Repeat("y", 5000)
	out := Render(ev(events.EventTypeBlocked, longSummary))
	if len(out) > 4096 {
		t.Errorf("expected output <= 4096 bytes, got %d", len(out))
	}
}
