package state

import (
	"strings"
	"time"
)

type SessionID string
type RunID string

type SessionStatus string
type SessionPhase string
type EscalationState string
type SupportState string
type RunStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusBlocked SessionStatus = "blocked"
	SessionStatusDone    SessionStatus = "done"
	SessionStatusFailed  SessionStatus = "failed"
)

const (
	SessionPhasePlanning     SessionPhase = "planning"
	SessionPhaseReadingCode  SessionPhase = "reading_code"
	SessionPhaseEditing      SessionPhase = "editing"
	SessionPhaseRunningTests SessionPhase = "running_tests"
	SessionPhaseWaiting      SessionPhase = "waiting"
)

const (
	EscalationStateNone   EscalationState = "none"
	EscalationStateNeeded EscalationState = "needed"
	EscalationStateActive EscalationState = "active"
)

const (
	SupportStateIdle      SupportState = "idle"
	SupportStateReplied   SupportState = "replied"
	SupportStateAskHuman  SupportState = "ask_human"
	SupportStateEscalated SupportState = "escalated"
)

const (
	RunStatusActive RunStatus = "active"
	RunStatusExited RunStatus = "exited"
	RunStatusFailed RunStatus = "failed"
)

type Session struct {
	ID                     SessionID         `json:"session_id"`
	ActiveRunID            RunID             `json:"run_id"`
	Provider               string            `json:"provider"`
	GeneralTopicID         int64             `json:"general_topic_id"`
	SessionTopicID         int64             `json:"session_topic_id"`
	SessionTopicTitle      string            `json:"session_topic_title,omitempty"`
	Status                 SessionStatus     `json:"status"`
	Phase                  SessionPhase      `json:"phase"`
	LastGoal               string            `json:"last_goal,omitempty"`
	LastQuestion           string            `json:"last_question,omitempty"`
	LastBlocker            string            `json:"last_blocker,omitempty"`
	RecentFilesSummary     string            `json:"recent_files_summary,omitempty"`
	RecentTestsSummary     string            `json:"recent_tests_summary,omitempty"`
	RecentEvents           []string          `json:"recent_events,omitempty"`
	ReplyAttemptCount      int               `json:"reply_attempt_count,omitempty"`
	LastRoleUsed           string            `json:"last_role_used,omitempty"`
	WorkRoot               string            `json:"work_root,omitempty"`
	SetupMessageID         int               `json:"setup_message_id,omitempty"`
	SessionHeaderMessageID int               `json:"session_header_message_id,omitempty"`
	SetupAwaiting          string            `json:"setup_awaiting,omitempty"`
	BrowseRootPath         string            `json:"browse_root_path,omitempty"`
	BrowseCurrentPath      string            `json:"browse_current_path,omitempty"`
	BrowsePage             int               `json:"browse_page,omitempty"`
	EscalationState        EscalationState   `json:"escalation_state"`
	SupportState           SupportState      `json:"support_state"`
	SupportDecisionSummary string            `json:"support_decision_summary,omitempty"`
	HumanActionNeeded      bool              `json:"human_action_needed"`
	LinkedDecisionRefs     []string          `json:"linked_decision_refs,omitempty"`
	PendingElevation       *PendingElevation `json:"pending_elevation,omitempty"`
	EvidenceRefs           []string          `json:"evidence_refs,omitempty"`
	OwnerUserID            int64             `json:"owner_user_id,omitempty"`
	LastHumanActorID       int64             `json:"last_human_actor_id,omitempty"`
	DeliveryState          map[string]string `json:"delivery_state,omitempty"`
}

type PendingElevation struct {
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Session) ApplySnapshot(snapshot Snapshot) {
	s.Status = SessionStatus(snapshot.Status)
	s.Phase = SessionPhase(snapshot.Phase)
	s.LastBlocker = snapshot.LastBlocker
	s.LastQuestion = snapshot.LastQuestion
	s.RecentFilesSummary = snapshot.RecentFilesSummary
	s.RecentTestsSummary = snapshot.RecentTestsSummary
	s.RecentEvents = append([]string(nil), snapshot.RecentEvents...)
	s.ReplyAttemptCount = snapshot.ReplyAttemptCount
	s.EscalationState = EscalationState(strings.TrimSpace(snapshot.EscalationState))
	s.SupportState = SupportState(strings.TrimSpace(snapshot.SupportState))
	if s.SupportState == "" {
		s.SupportState = SupportStateIdle
	}
	s.SupportDecisionSummary = snapshot.SupportDecisionSummary
	s.HumanActionNeeded = snapshot.HumanActionNeeded
}

type Run struct {
	ID                RunID     `json:"run_id"`
	SessionID         SessionID `json:"session_id"`
	Provider          string    `json:"provider"`
	ProviderSessionID string    `json:"provider_session_id,omitempty"`
	ProcessID         int       `json:"process_id,omitempty"`
	TranscriptPath    string    `json:"transcript_path,omitempty"`
	Status            RunStatus `json:"status"`
	StartedAt         time.Time `json:"started_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}
