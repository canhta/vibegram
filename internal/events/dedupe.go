package events

import "strings"

type Deduper struct {
	seen map[string]struct{}
}

func NewDeduper() *Deduper {
	return &Deduper{seen: make(map[string]struct{})}
}

func (d *Deduper) MarkIfNew(event NormalizedEvent) bool {
	if d == nil {
		return false
	}

	keys := []string{event.DeliveryKey}
	if semantic := semanticDedupKey(event); semantic != "" {
		keys = append(keys, semantic)
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := d.seen[key]; ok {
			return false
		}
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		d.seen[key] = struct{}{}
	}
	return true
}

func semanticDedupKey(event NormalizedEvent) string {
	summary := strings.TrimSpace(event.Summary)
	if summary == "" {
		return ""
	}

	switch event.EventType {
	case EventTypeQuestion, EventTypeBlocked, EventTypeApprovalNeeded, EventTypeBlockerResolved, EventTypeDone, EventTypeFailed:
		return "semantic:" + string(event.EventType) + ":" + string(event.RunID) + ":" + shortHash(summary)
	default:
		return ""
	}
}
