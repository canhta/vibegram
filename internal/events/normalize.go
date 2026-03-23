package events

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

var rawTypeToEventType = map[string]EventType{
	"session_started":  EventTypeSessionStarted,
	"phase_changed":    EventTypePhaseChanged,
	"tool_activity":    EventTypeToolActivity,
	"files_changed":    EventTypeFilesChanged,
	"tests_changed":    EventTypeTestsChanged,
	"question":         EventTypeQuestion,
	"blocked":          EventTypeBlocked,
	"blocker_resolved": EventTypeBlockerResolved,
	"approval_needed":  EventTypeApprovalNeeded,
	"agent_reply_sent": EventTypeAgentReplySent,
	"done":             EventTypeDone,
	"failed":           EventTypeFailed,
}

func Normalize(observation Observation) (NormalizedEvent, error) {
	eventType, ok := rawTypeToEventType[strings.TrimSpace(observation.RawType)]
	if !ok {
		return NormalizedEvent{}, fmt.Errorf("unsupported raw_type %q", observation.RawType)
	}
	if observation.SessionID == "" {
		return NormalizedEvent{}, fmt.Errorf("session_id is required")
	}
	if observation.RunID == "" {
		return NormalizedEvent{}, fmt.Errorf("run_id is required")
	}
	if observation.Provider == "" {
		return NormalizedEvent{}, fmt.Errorf("provider is required")
	}
	if observation.Source == "" {
		return NormalizedEvent{}, fmt.Errorf("source is required")
	}
	if observation.RawTimestamp.IsZero() {
		return NormalizedEvent{}, fmt.Errorf("raw_timestamp is required")
	}
	if strings.TrimSpace(observation.Summary) == "" {
		return NormalizedEvent{}, fmt.Errorf("summary is required")
	}

	deliveryKey := buildDeliveryKey(observation.SessionID, eventType, observation.ProviderID, observation.RawTimestamp.String())
	eventID := "evt_" + shortHash(observation.SessionID, observation.RunID, string(eventType), observation.ProviderID, observation.RawTimestamp.UTC().Format(timeFormat))

	return NormalizedEvent{
		EventID:     eventID,
		SessionID:   observation.SessionID,
		RunID:       observation.RunID,
		Provider:    observation.Provider,
		EventType:   eventType,
		SourceClass: sourceClassFor(observation.Source),
		DeliveryKey: deliveryKey,
		Severity:    severityFor(eventType),
		Timestamp:   observation.RawTimestamp.UTC(),
		Summary:     observation.Summary,
		Details:     cloneMap(observation.Details),
		Sources: []EventSource{{
			Source:       observation.Source,
			ProviderType: observation.ProviderType,
			ProviderID:   observation.ProviderID,
		}},
		ArtifactRefs: append([]string(nil), observation.ArtifactRefs...),
	}, nil
}

const timeFormat = "20060102T150405Z"

func buildDeliveryKey(sessionID string, eventType EventType, providerID string, fallback string) string {
	suffix := providerID
	if strings.TrimSpace(suffix) == "" {
		suffix = shortHash(fallback)
	}
	return fmt.Sprintf("%s:%s:%s", sessionID, eventType, suffix)
}

func shortHash(parts ...string) string {
	hash := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(hash[:])[:12]
}

func sourceClassFor(source Source) SourceClass {
	switch source {
	case SourceHook, SourceTranscript, SourcePTY:
		return SourceClassUntrustedEvidence
	default:
		return SourceClassUntrustedEvidence
	}
}

func severityFor(eventType EventType) Severity {
	switch eventType {
	case EventTypeBlocked, EventTypeApprovalNeeded:
		return SeverityWarning
	case EventTypeFailed:
		return SeverityError
	default:
		return SeverityInfo
	}
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
