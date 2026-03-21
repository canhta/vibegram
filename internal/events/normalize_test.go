package events_test

import (
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
)

func TestNormalizeMapsObservationToBlockedEvent(t *testing.T) {
	now := time.Date(2026, time.March, 21, 14, 0, 0, 0, time.UTC)

	observation := events.Observation{
		SessionID:    "ses_123",
		RunID:        "run_123",
		Provider:     events.ProviderCodex,
		Source:       events.SourceTranscript,
		RawType:      "blocked",
		RawTimestamp: now,
		Summary:      "Agent is blocked on a missing environment variable.",
		Details: map[string]any{
			"question":        "Should I use STAGING_API_KEY or create a new token?",
			"safe_auto_reply": false,
		},
		ProviderType: "response_item",
		ProviderID:   "abc123",
	}

	got, err := events.Normalize(observation)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if got.EventType != events.EventTypeBlocked {
		t.Fatalf("EventType = %q, want %q", got.EventType, events.EventTypeBlocked)
	}
	if got.SourceClass != events.SourceClassUntrustedEvidence {
		t.Fatalf("SourceClass = %q, want %q", got.SourceClass, events.SourceClassUntrustedEvidence)
	}
	if got.DeliveryKey != "ses_123:blocked:abc123" {
		t.Fatalf("DeliveryKey = %q, want %q", got.DeliveryKey, "ses_123:blocked:abc123")
	}
	if len(got.Sources) != 1 || got.Sources[0].ProviderID != "abc123" {
		t.Fatalf("Sources = %#v, want provider id abc123", got.Sources)
	}
}

func TestNormalizeSupportsBalancedEventSet(t *testing.T) {
	tests := []struct {
		rawType string
		want    events.EventType
	}{
		{rawType: "session_started", want: events.EventTypeSessionStarted},
		{rawType: "phase_changed", want: events.EventTypePhaseChanged},
		{rawType: "tool_activity", want: events.EventTypeToolActivity},
		{rawType: "files_changed", want: events.EventTypeFilesChanged},
		{rawType: "tests_changed", want: events.EventTypeTestsChanged},
		{rawType: "question", want: events.EventTypeQuestion},
		{rawType: "blocked", want: events.EventTypeBlocked},
		{rawType: "approval_needed", want: events.EventTypeApprovalNeeded},
		{rawType: "agent_reply_sent", want: events.EventTypeAgentReplySent},
		{rawType: "done", want: events.EventTypeDone},
		{rawType: "failed", want: events.EventTypeFailed},
	}

	for _, tt := range tests {
		t.Run(tt.rawType, func(t *testing.T) {
			got, err := events.Normalize(events.Observation{
				SessionID:    "ses_001",
				RunID:        "run_001",
				Provider:     events.ProviderClaude,
				Source:       events.SourceHook,
				RawType:      tt.rawType,
				RawTimestamp: time.Date(2026, time.March, 21, 15, 0, 0, 0, time.UTC),
				Summary:      tt.rawType + " summary",
				ProviderID:   tt.rawType + "-id",
			})
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if got.EventType != tt.want {
				t.Fatalf("EventType = %q, want %q", got.EventType, tt.want)
			}
		})
	}
}

func TestDeduperSuppressesReplayOfSameLogicalEvent(t *testing.T) {
	deduper := events.NewDeduper()
	event := events.NormalizedEvent{
		SessionID:   "ses_123",
		EventType:   events.EventTypeBlocked,
		DeliveryKey: "ses_123:blocked:abc123",
		SourceClass: events.SourceClassUntrustedEvidence,
		Summary:     "Blocked on missing env var",
		EventID:     "evt_1",
		RunID:       "run_123",
		Provider:    events.ProviderCodex,
		Severity:    events.SeverityWarning,
		Timestamp:   time.Date(2026, time.March, 21, 16, 0, 0, 0, time.UTC),
	}

	if !deduper.MarkIfNew(event) {
		t.Fatal("first MarkIfNew() = false, want true")
	}
	if deduper.MarkIfNew(event) {
		t.Fatal("second MarkIfNew() = true, want false")
	}
}
