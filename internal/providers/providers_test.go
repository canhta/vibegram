package providers_test

import (
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/providers"
)

func TestClaudePriorityPrefersHookThenTranscriptThenPTY(t *testing.T) {
	picker := providers.NewPriorityPicker(events.ProviderClaude)
	baseTime := time.Date(2026, time.March, 21, 17, 0, 0, 0, time.UTC)

	observations := []events.Observation{
		{
			SessionID:    "ses_1",
			RunID:        "run_1",
			Provider:     events.ProviderClaude,
			Source:       events.SourcePTY,
			RawType:      "blocked",
			RawTimestamp: baseTime,
			Summary:      "blocked",
			ProviderID:   "same-event",
		},
		{
			SessionID:    "ses_1",
			RunID:        "run_1",
			Provider:     events.ProviderClaude,
			Source:       events.SourceTranscript,
			RawType:      "blocked",
			RawTimestamp: baseTime,
			Summary:      "blocked",
			ProviderID:   "same-event",
		},
		{
			SessionID:    "ses_1",
			RunID:        "run_1",
			Provider:     events.ProviderClaude,
			Source:       events.SourceHook,
			RawType:      "blocked",
			RawTimestamp: baseTime,
			Summary:      "blocked",
			ProviderID:   "same-event",
		},
	}

	got, ok := picker.Preferred(observations)
	if !ok {
		t.Fatal("Preferred() ok = false, want true")
	}
	if got.Source != events.SourceHook {
		t.Fatalf("Source = %q, want %q", got.Source, events.SourceHook)
	}
}

func TestCodexPriorityPrefersTranscriptThenPTY(t *testing.T) {
	picker := providers.NewPriorityPicker(events.ProviderCodex)
	baseTime := time.Date(2026, time.March, 21, 17, 30, 0, 0, time.UTC)

	observations := []events.Observation{
		{
			SessionID:    "ses_2",
			RunID:        "run_2",
			Provider:     events.ProviderCodex,
			Source:       events.SourcePTY,
			RawType:      "question",
			RawTimestamp: baseTime,
			Summary:      "question",
			ProviderID:   "same-event",
		},
		{
			SessionID:    "ses_2",
			RunID:        "run_2",
			Provider:     events.ProviderCodex,
			Source:       events.SourceTranscript,
			RawType:      "question",
			RawTimestamp: baseTime,
			Summary:      "question",
			ProviderID:   "same-event",
		},
	}

	got, ok := picker.Preferred(observations)
	if !ok {
		t.Fatal("Preferred() ok = false, want true")
	}
	if got.Source != events.SourceTranscript {
		t.Fatalf("Source = %q, want %q", got.Source, events.SourceTranscript)
	}
}

func TestDistinctRawObservationsRemainDistinguishableBeforeNormalization(t *testing.T) {
	observationA := providers.RawObservation{
		Observation: events.Observation{
			SessionID:    "ses_3",
			RunID:        "run_3",
			Provider:     events.ProviderClaude,
			Source:       events.SourceHook,
			RawType:      "tool_activity",
			RawTimestamp: time.Date(2026, time.March, 21, 18, 0, 0, 0, time.UTC),
			Summary:      "Ran command",
			ProviderID:   "evt_a",
		},
	}
	observationB := providers.RawObservation{
		Observation: events.Observation{
			SessionID:    "ses_3",
			RunID:        "run_3",
			Provider:     events.ProviderClaude,
			Source:       events.SourceTranscript,
			RawType:      "tool_activity",
			RawTimestamp: time.Date(2026, time.March, 21, 18, 0, 1, 0, time.UTC),
			Summary:      "Ran command",
			ProviderID:   "evt_b",
		},
	}

	if observationA.IdentityKey() == observationB.IdentityKey() {
		t.Fatalf("IdentityKey() values matched, want distinct keys: %q", observationA.IdentityKey())
	}
}
