package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/state"
)

func TestStoreSaveAndLoadSession(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)

	session := state.Session{
		ID:                "ses_123",
		ActiveRunID:       "run_123",
		Provider:          "codex",
		GeneralTopicID:    1,
		SessionTopicID:    42,
		SessionTopicTitle: "project-x codex 0123",
		Status:            state.SessionStatusRunning,
		Phase:             state.SessionPhasePlanning,
		LastGoal:          "Bootstrap the daemon",
		EscalationState:   state.EscalationStateNone,
		OwnerUserID:       1001,
		LastHumanActorID:  1001,
		ReplyAttemptCount: 1,
		RecentEvents:      []string{"session_started"},
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	sessionPath := filepath.Join(root, "sessions", "ses_123.json")
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("session file stat error = %v", err)
	}

	got, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}

	if got.Status != state.SessionStatusRunning {
		t.Fatalf("Status = %q, want %q", got.Status, state.SessionStatusRunning)
	}
	if got.Phase != state.SessionPhasePlanning {
		t.Fatalf("Phase = %q, want %q", got.Phase, state.SessionPhasePlanning)
	}
	if got.SessionTopicID != 42 {
		t.Fatalf("SessionTopicID = %d, want 42", got.SessionTopicID)
	}
	if got.SessionTopicTitle != "project-x codex 0123" {
		t.Fatalf("SessionTopicTitle = %q, want %q", got.SessionTopicTitle, "project-x codex 0123")
	}
}

func TestStorePersistsSessionUpdatesAcrossRestart(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)

	session := state.Session{
		ID:              "ses_456",
		ActiveRunID:     "run_456",
		Provider:        "claude",
		GeneralTopicID:  1,
		SessionTopicID:  99,
		Status:          state.SessionStatusRunning,
		Phase:           state.SessionPhaseReadingCode,
		EscalationState: state.EscalationStateNone,
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	session.Status = state.SessionStatusBlocked
	session.Phase = state.SessionPhaseWaiting
	session.LastBlocker = "Missing TELEGRAM_BOT_TOKEN"
	session.EscalationState = state.EscalationStateNeeded
	session.ReplyAttemptCount = 2

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() update error = %v", err)
	}

	restarted := state.NewStore(root)
	got, err := restarted.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession() after restart error = %v", err)
	}

	if got.Status != state.SessionStatusBlocked {
		t.Fatalf("Status = %q, want %q", got.Status, state.SessionStatusBlocked)
	}
	if got.Phase != state.SessionPhaseWaiting {
		t.Fatalf("Phase = %q, want %q", got.Phase, state.SessionPhaseWaiting)
	}
	if got.LastBlocker != "Missing TELEGRAM_BOT_TOKEN" {
		t.Fatalf("LastBlocker = %q, want %q", got.LastBlocker, "Missing TELEGRAM_BOT_TOKEN")
	}
	if got.EscalationState != state.EscalationStateNeeded {
		t.Fatalf("EscalationState = %q, want %q", got.EscalationState, state.EscalationStateNeeded)
	}
}

func TestStoreSaveAndLoadSessionPersistsSupportFields(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)

	session := state.Session{
		ID:                     "ses_support",
		ActiveRunID:            "run_support",
		Provider:               "codex",
		GeneralTopicID:         1,
		SessionTopicID:         77,
		Status:                 state.SessionStatusBlocked,
		Phase:                  state.SessionPhaseWaiting,
		SupportState:           state.SupportStateAskHuman,
		SupportDecisionSummary: "which file should I edit?",
		HumanActionNeeded:      true,
		SessionHeaderMessageID: 9021,
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	got, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}

	if got.SupportState != state.SupportStateAskHuman {
		t.Fatalf("SupportState = %q, want %q", got.SupportState, state.SupportStateAskHuman)
	}
	if got.SupportDecisionSummary != "which file should I edit?" {
		t.Fatalf("SupportDecisionSummary = %q, want %q", got.SupportDecisionSummary, "which file should I edit?")
	}
	if !got.HumanActionNeeded {
		t.Fatal("HumanActionNeeded = false, want true")
	}
	if got.SessionHeaderMessageID != 9021 {
		t.Fatalf("SessionHeaderMessageID = %d, want 9021", got.SessionHeaderMessageID)
	}
}

func TestStoreSaveAndLoadRun(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	now := time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC)

	run := state.Run{
		ID:                "run_789",
		SessionID:         "ses_789",
		Provider:          "codex",
		ProviderSessionID: "provider_789",
		ProcessID:         4242,
		TranscriptPath:    "/tmp/transcript.jsonl",
		Status:            state.RunStatusActive,
		StartedAt:         now,
		UpdatedAt:         now,
	}

	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	runPath := filepath.Join(root, "runs", "run_789.json")
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("run file stat error = %v", err)
	}

	got, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}

	if got.ProviderSessionID != "provider_789" {
		t.Fatalf("ProviderSessionID = %q, want %q", got.ProviderSessionID, "provider_789")
	}
	if got.ProcessID != 4242 {
		t.Fatalf("ProcessID = %d, want 4242", got.ProcessID)
	}
	if !got.StartedAt.Equal(now) {
		t.Fatalf("StartedAt = %s, want %s", got.StartedAt, now)
	}
}

func TestStoreLoadSessionReturnsNotFound(t *testing.T) {
	store := state.NewStore(t.TempDir())

	_, err := store.LoadSession("ses_missing")
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("LoadSession() error = %v, want ErrNotFound", err)
	}
}

func TestStoreSaveAndLoadCursor(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := store.SaveCursor("telegram_updates", 123); err != nil {
		t.Fatalf("SaveCursor() error = %v", err)
	}

	got, err := store.LoadCursor("telegram_updates")
	if err != nil {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if got != 123 {
		t.Fatalf("LoadCursor() = %d, want 123", got)
	}
}

func TestStoreLoadCursorReturnsNotFound(t *testing.T) {
	store := state.NewStore(t.TempDir())

	_, err := store.LoadCursor("telegram_updates")
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("LoadCursor() error = %v, want ErrNotFound", err)
	}
}

func TestStoreListSessionsAndDeleteSessionAndRun(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sessionA := state.Session{ID: "ses_a", ActiveRunID: "run_a", Provider: "codex", GeneralTopicID: 1, SessionTopicID: 12, Status: state.SessionStatusRunning, Phase: state.SessionPhasePlanning}
	sessionB := state.Session{ID: "ses_b", ActiveRunID: "run_b", Provider: "codex", GeneralTopicID: 1, SessionTopicID: 14, Status: state.SessionStatusRunning, Phase: state.SessionPhasePlanning}
	runA := state.Run{ID: "run_a", SessionID: "ses_a", Provider: "codex", Status: state.RunStatusExited}

	if err := store.SaveSession(sessionA); err != nil {
		t.Fatalf("SaveSession(sessionA) error = %v", err)
	}
	if err := store.SaveSession(sessionB); err != nil {
		t.Fatalf("SaveSession(sessionB) error = %v", err)
	}
	if err := store.SaveRun(runA); err != nil {
		t.Fatalf("SaveRun(runA) error = %v", err)
	}

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(ListSessions()) = %d, want 2", len(sessions))
	}

	if err := store.DeleteSession("ses_a"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if err := store.DeleteRun("run_a"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}

	if _, err := store.LoadSession("ses_a"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("LoadSession() error = %v, want ErrNotFound after delete", err)
	}
	if _, err := store.LoadRun("run_a"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("LoadRun() error = %v, want ErrNotFound after delete", err)
	}
}
