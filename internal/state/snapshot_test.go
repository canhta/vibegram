package state

import (
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
)

func makeEvent(et events.EventType, summary string) events.NormalizedEvent {
	return events.NormalizedEvent{
		EventID:   "evt-test",
		EventType: et,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

func TestSnapshotApplyBlockedSetsLastBlocker(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeBlocked, "missing file path"))
	if s.LastBlocker != "missing file path" {
		t.Errorf("expected LastBlocker to be set, got %q", s.LastBlocker)
	}
}

func TestSnapshotApplyQuestionSetsLastQuestion(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeQuestion, "which test framework?"))
	if s.LastQuestion != "which test framework?" {
		t.Errorf("expected LastQuestion to be set, got %q", s.LastQuestion)
	}
}

func TestSnapshotApplyPhaseChangedSetsPhase(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypePhaseChanged, "editing"))
	if s.Phase != "editing" {
		t.Errorf("expected Phase=editing, got %q", s.Phase)
	}
}

func TestSnapshotApplyFilesChangedSetsSummary(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeFilesChanged, "main.go updated"))
	if s.RecentFilesSummary != "main.go updated" {
		t.Errorf("expected RecentFilesSummary to be set, got %q", s.RecentFilesSummary)
	}
}

func TestSnapshotApplyTestsChangedSetsSummary(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeTestsChanged, "all tests passed"))
	if s.RecentTestsSummary != "all tests passed" {
		t.Errorf("expected RecentTestsSummary to be set, got %q", s.RecentTestsSummary)
	}
}

func TestSnapshotApplyDoneSetsStatusDone(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeDone, "task complete"))
	if s.Status != "done" {
		t.Errorf("expected Status=done, got %q", s.Status)
	}
}

func TestSnapshotApplyFailedSetsStatusFailed(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeFailed, "build error"))
	if s.Status != "failed" {
		t.Errorf("expected Status=failed, got %q", s.Status)
	}
}

func TestSnapshotRecentEventsBounded(t *testing.T) {
	s := &Snapshot{}
	for i := 0; i < 25; i++ {
		s.Apply(makeEvent(events.EventTypeToolActivity, "tool run"))
	}
	if len(s.RecentEvents) > 20 {
		t.Errorf("expected at most 20 recent events, got %d", len(s.RecentEvents))
	}
}

func TestSnapshotApplyBlockedIncrementsReplyAttemptCount(t *testing.T) {
	s := &Snapshot{}
	s.Apply(makeEvent(events.EventTypeBlocked, "err1"))
	if s.ReplyAttemptCount != 1 {
		t.Errorf("expected ReplyAttemptCount=1, got %d", s.ReplyAttemptCount)
	}
	s.Apply(makeEvent(events.EventTypeBlocked, "err2"))
	if s.ReplyAttemptCount != 2 {
		t.Errorf("expected ReplyAttemptCount=2, got %d", s.ReplyAttemptCount)
	}
}

func TestSnapshotApplyBlockerResolvedClearsBlockerState(t *testing.T) {
	s := &Snapshot{
		LastBlocker:       "missing token",
		ReplyAttemptCount: 3,
		EscalationState:   "needed",
	}
	s.Apply(makeEvent(events.EventTypeBlockerResolved, "token added"))
	if s.LastBlocker != "" {
		t.Errorf("expected LastBlocker to be cleared, got %q", s.LastBlocker)
	}
	if s.ReplyAttemptCount != 0 {
		t.Errorf("expected ReplyAttemptCount=0, got %d", s.ReplyAttemptCount)
	}
	if s.EscalationState != "" {
		t.Errorf("expected EscalationState to be cleared, got %q", s.EscalationState)
	}
}

func TestSnapshotApplyNonBlockedResetsReplyAttemptCount(t *testing.T) {
	s := &Snapshot{ReplyAttemptCount: 5}
	s.Apply(makeEvent(events.EventTypePhaseChanged, "editing"))
	if s.ReplyAttemptCount != 0 {
		t.Errorf("phase_changed should reset ReplyAttemptCount, got %d", s.ReplyAttemptCount)
	}

	s.ReplyAttemptCount = 5
	s.Apply(makeEvent(events.EventTypeToolActivity, "ran tool"))
	if s.ReplyAttemptCount != 0 {
		t.Errorf("tool_activity should reset ReplyAttemptCount, got %d", s.ReplyAttemptCount)
	}
}

func TestSnapshotApplySupportFallbackMarksAskHuman(t *testing.T) {
	s := &Snapshot{}
	event := makeEvent(events.EventTypeQuestion, "which file should I edit?")

	s.Apply(event)
	s.ApplySupportFallback(event)

	if s.SupportState != string(SupportStateAskHuman) {
		t.Fatalf("SupportState = %q, want %q", s.SupportState, SupportStateAskHuman)
	}
	if s.SupportDecisionSummary != "which file should I edit?" {
		t.Fatalf("SupportDecisionSummary = %q, want %q", s.SupportDecisionSummary, "which file should I edit?")
	}
	if !s.HumanActionNeeded {
		t.Fatal("HumanActionNeeded = false, want true")
	}
}

func TestSnapshotApplySupportDecisionMarksReply(t *testing.T) {
	s := &Snapshot{}

	s.ApplySupportDecision(SupportStateReplied, "Use Go's testing package", false)

	if s.SupportState != string(SupportStateReplied) {
		t.Fatalf("SupportState = %q, want %q", s.SupportState, SupportStateReplied)
	}
	if s.SupportDecisionSummary != "Use Go's testing package" {
		t.Fatalf("SupportDecisionSummary = %q, want %q", s.SupportDecisionSummary, "Use Go's testing package")
	}
	if s.HumanActionNeeded {
		t.Fatal("HumanActionNeeded = true, want false")
	}
}

func TestSnapshotPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	snap := Snapshot{
		SessionID:          "sess-123",
		Phase:              "editing",
		Status:             "running",
		LastBlocker:        "missing import",
		LastQuestion:       "which framework?",
		RecentFilesSummary: "main.go",
		RecentTestsSummary: "2 passed",
		RecentEvents:       []string{"started", "editing"},
		ReplyAttemptCount:  2,
		EscalationState:    "needed",
		UpdatedAt:          time.Now().UTC().Truncate(time.Second),
	}

	if err := store.SaveSnapshot("sess-123", snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := store.LoadSnapshot("sess-123")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	if loaded.SessionID != snap.SessionID {
		t.Errorf("SessionID: got %q, want %q", loaded.SessionID, snap.SessionID)
	}
	if loaded.Phase != snap.Phase {
		t.Errorf("Phase: got %q, want %q", loaded.Phase, snap.Phase)
	}
	if loaded.LastBlocker != snap.LastBlocker {
		t.Errorf("LastBlocker: got %q, want %q", loaded.LastBlocker, snap.LastBlocker)
	}
	if loaded.ReplyAttemptCount != snap.ReplyAttemptCount {
		t.Errorf("ReplyAttemptCount: got %d, want %d", loaded.ReplyAttemptCount, snap.ReplyAttemptCount)
	}
	if loaded.EscalationState != snap.EscalationState {
		t.Errorf("EscalationState: got %q, want %q", loaded.EscalationState, snap.EscalationState)
	}
	if len(loaded.RecentEvents) != len(snap.RecentEvents) {
		t.Errorf("RecentEvents len: got %d, want %d", len(loaded.RecentEvents), len(snap.RecentEvents))
	}
	if !strings.Contains(strings.Join(loaded.RecentEvents, ","), "started") {
		t.Errorf("RecentEvents should contain 'started'")
	}
}
