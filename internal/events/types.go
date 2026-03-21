package events

import "time"

type Provider string
type Source string
type SourceClass string
type EventType string
type Severity string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

const (
	SourceHook       Source = "hook"
	SourceTranscript Source = "transcript"
	SourcePTY        Source = "pty"
)

const (
	SourceClassTrustedPolicy     SourceClass = "trusted_policy"
	SourceClassTrustedSystem     SourceClass = "trusted_system"
	SourceClassUntrustedEvidence SourceClass = "untrusted_evidence"
)

const (
	EventTypeSessionStarted EventType = "session_started"
	EventTypePhaseChanged   EventType = "phase_changed"
	EventTypeToolActivity   EventType = "tool_activity"
	EventTypeFilesChanged   EventType = "files_changed"
	EventTypeTestsChanged   EventType = "tests_changed"
	EventTypeQuestion       EventType = "question"
	EventTypeBlocked        EventType = "blocked"
	EventTypeApprovalNeeded EventType = "approval_needed"
	EventTypeAgentReplySent EventType = "agent_reply_sent"
	EventTypeDone           EventType = "done"
	EventTypeFailed         EventType = "failed"
)

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

type Observation struct {
	SessionID    string
	RunID        string
	Provider     Provider
	Source       Source
	RawType      string
	RawTimestamp time.Time
	Summary      string
	Details      map[string]any
	ProviderType string
	ProviderID   string
	ArtifactRefs []string
}

type EventSource struct {
	Source       Source `json:"source"`
	ProviderType string `json:"provider_type,omitempty"`
	ProviderID   string `json:"provider_id,omitempty"`
}

type NormalizedEvent struct {
	EventID      string         `json:"event_id"`
	SessionID    string         `json:"session_id"`
	RunID        string         `json:"run_id"`
	Provider     Provider       `json:"provider"`
	EventType    EventType      `json:"event_type"`
	SourceClass  SourceClass    `json:"source_class"`
	DeliveryKey  string         `json:"delivery_key"`
	Severity     Severity       `json:"severity"`
	Timestamp    time.Time      `json:"timestamp"`
	Summary      string         `json:"summary"`
	Details      map[string]any `json:"details,omitempty"`
	Sources      []EventSource  `json:"sources,omitempty"`
	ArtifactRefs []string       `json:"artifact_refs,omitempty"`
}
