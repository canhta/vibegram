package telegram

import (
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
)

func makeRouter() Router {
	return Router{
		ChatID:          100,
		GeneralThreadID: 1,
		SessionThreadID: 42,
	}
}

func routerEv(et events.EventType) events.NormalizedEvent {
	return events.NormalizedEvent{
		EventType: et,
		Timestamp: time.Now(),
	}
}

func assertDestinations(t *testing.T, got []Destination, wantTypes []TopicType) {
	t.Helper()
	if len(got) != len(wantTypes) {
		t.Fatalf("expected %d destinations, got %d: %+v", len(wantTypes), len(got), got)
	}
	for i, d := range got {
		if d.Type != wantTypes[i] {
			t.Errorf("dest[%d]: expected type %v, got %v", i, wantTypes[i], d.Type)
		}
	}
}

func TestRouteToolActivityToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeToolActivity))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRoutePhaseChangedToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypePhaseChanged))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRouteFilesChangedToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeFilesChanged))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRouteTestsChangedToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeTestsChanged))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRouteQuestionToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeQuestion))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRouteBlockedToBothTopics(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeBlocked))
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations for blocked, got %d", len(dests))
	}
}

func TestRouteBlockerResolvedToBothTopics(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeBlockerResolved))
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations for blocker_resolved, got %d", len(dests))
	}
}

func TestRouteDoneToBothTopics(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeDone))
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations for done, got %d", len(dests))
	}
}

func TestRouteFailedToBothTopics(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeFailed))
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations for failed, got %d", len(dests))
	}
}

func TestRouteApprovalNeededToBothTopics(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeApprovalNeeded))
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations for approval_needed, got %d", len(dests))
	}
}

func TestRouteSessionStartedToGeneralTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeSessionStarted))
	assertDestinations(t, dests, []TopicType{TopicGeneral})
}

func TestRouteAgentReplySentToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(routerEv(events.EventTypeAgentReplySent))
	assertDestinations(t, dests, []TopicType{TopicSession})
}

func TestRouteUnknownEventToSessionTopicOnly(t *testing.T) {
	r := makeRouter()
	dests := r.Route(events.NormalizedEvent{EventType: "unknown_type", Timestamp: time.Now()})
	assertDestinations(t, dests, []TopicType{TopicSession})
}
