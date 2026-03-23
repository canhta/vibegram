package telegram

import (
	"fmt"
	"strings"

	"github.com/canhta/vibegram/internal/events"
)

const (
	maxSummaryLen = 200
	maxMessageLen = 4096
)

func Render(event events.NormalizedEvent) string {
	summary := event.Summary
	limit := summaryLimit(event.EventType)
	summary = TruncateText(summary, limit)

	var text string
	switch event.EventType {
	case events.EventTypeSessionStarted:
		text = fmt.Sprintf("Session started: %s", summary)
	case events.EventTypeBlocked:
		text = fmt.Sprintf("Blocked: %s", summary)
	case events.EventTypeBlockerResolved:
		text = fmt.Sprintf("Blocker resolved: %s", summary)
	case events.EventTypeQuestion:
		text = fmt.Sprintf("Question: %s", summary)
	case events.EventTypeDone:
		text = fmt.Sprintf("Done: %s", summary)
	case events.EventTypeFailed:
		text = fmt.Sprintf("Failed: %s", summary)
	case events.EventTypeFilesChanged:
		text = fmt.Sprintf("Files: %s", summary)
	case events.EventTypeTestsChanged:
		text = fmt.Sprintf("Tests: %s", summary)
	case events.EventTypeApprovalNeeded:
		text = fmt.Sprintf("Approval needed: %s", summary)
	case events.EventTypePhaseChanged:
		text = fmt.Sprintf("Phase: %s", summary)
	case events.EventTypeAgentReplySent:
		text = fmt.Sprintf("Agent reply: %s", summary)
	default:
		text = summary
	}

	return ClampMessageText(text)
}

func summaryLimit(eventType events.EventType) int {
	if eventType == events.EventTypeQuestion {
		return maxMessageLen
	}
	return maxSummaryLen
}

func ClampMessageText(text string) string {
	return TruncateText(text, maxMessageLen)
}

func TruncateText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || text == "" {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
