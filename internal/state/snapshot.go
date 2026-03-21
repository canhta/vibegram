package state

import (
	"time"

	"github.com/canhta/vibegram/internal/events"
)

const maxRecentEvents = 20

type Snapshot struct {
	SessionID          string    `json:"session_id"`
	Phase              string    `json:"phase"`
	Status             string    `json:"status"`
	LastBlocker        string    `json:"last_blocker,omitempty"`
	LastQuestion       string    `json:"last_question,omitempty"`
	RecentFilesSummary string    `json:"recent_files_summary,omitempty"`
	RecentTestsSummary string    `json:"recent_tests_summary,omitempty"`
	RecentEvents       []string  `json:"recent_events,omitempty"`
	ReplyAttemptCount  int       `json:"reply_attempt_count,omitempty"`
	EscalationState    string    `json:"escalation_state,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (s *Snapshot) Apply(event events.NormalizedEvent) {
	s.UpdatedAt = event.Timestamp

	switch event.EventType {
	case events.EventTypePhaseChanged:
		s.Phase = event.Summary
		s.ReplyAttemptCount = 0
	case events.EventTypeBlocked:
		s.LastBlocker = event.Summary
		s.ReplyAttemptCount++
		if s.ReplyAttemptCount >= 3 {
			s.EscalationState = "needed"
		}
	case events.EventTypeQuestion:
		s.LastQuestion = event.Summary
	case events.EventTypeFilesChanged:
		s.RecentFilesSummary = event.Summary
	case events.EventTypeTestsChanged:
		s.RecentTestsSummary = event.Summary
	case events.EventTypeDone:
		s.Status = "done"
	case events.EventTypeFailed:
		s.Status = "failed"
	case events.EventTypeToolActivity, events.EventTypeSessionStarted:
		s.ReplyAttemptCount = 0
	}

	s.RecentEvents = append(s.RecentEvents, event.Summary)
	if len(s.RecentEvents) > maxRecentEvents {
		s.RecentEvents = s.RecentEvents[len(s.RecentEvents)-maxRecentEvents:]
	}
}
