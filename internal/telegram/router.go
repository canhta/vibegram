package telegram

import "github.com/canhta/vibegram/internal/events"

type TopicType int

const (
	TopicGeneral TopicType = iota
	TopicSession
)

type Destination struct {
	Type     TopicType
	ChatID   int64
	ThreadID int
}

type Router struct {
	ChatID          int64
	GeneralThreadID int
	SessionThreadID int
}

func (r Router) Route(event events.NormalizedEvent) []Destination {
	general := Destination{Type: TopicGeneral, ChatID: r.ChatID, ThreadID: r.GeneralThreadID}
	session := Destination{Type: TopicSession, ChatID: r.ChatID, ThreadID: r.SessionThreadID}

	switch event.EventType {
	case events.EventTypeSessionStarted:
		return []Destination{general}
	case events.EventTypeBlocked, events.EventTypeBlockerResolved, events.EventTypeDone, events.EventTypeFailed, events.EventTypeApprovalNeeded:
		return []Destination{general, session}
	default:
		return []Destination{session}
	}
}
