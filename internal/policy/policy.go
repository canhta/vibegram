package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
)

const defaultMaxAutoReplies = 2

// RoleExecutor is the interface the policy uses to invoke the support role.
type RoleExecutor interface {
	Decide(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (roles.Decision, error)
}

type PolicyDecision struct {
	Action  roles.Action
	Message string
	Reason  string
}

type Engine struct {
	executor          RoleExecutor
	maxAutoReplyCount int
}

func NewEngine(executor RoleExecutor) *Engine {
	return &Engine{
		executor:          executor,
		maxAutoReplyCount: defaultMaxAutoReplies,
	}
}

func (e *Engine) Evaluate(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (PolicyDecision, error) {
	switch event.EventType {
	case events.EventTypeToolActivity,
		events.EventTypePhaseChanged,
		events.EventTypeFilesChanged,
		events.EventTypeTestsChanged,
		events.EventTypeSessionStarted,
		events.EventTypeBlockerResolved,
		events.EventTypeAgentReplySent:
		return PolicyDecision{Action: roles.ActionNoop}, nil

	case events.EventTypeDone:
		return PolicyDecision{Action: roles.ActionNoop}, nil

	case events.EventTypeApprovalNeeded:
		return PolicyDecision{Action: roles.ActionEscalate, Reason: "approval required"}, nil

	case events.EventTypeFailed:
		return PolicyDecision{Action: roles.ActionEscalate, Reason: "agent failed"}, nil

	case events.EventTypeBlocked:
		if isRepeatedSupportEvent(snap, event) {
			return PolicyDecision{Action: roles.ActionNoop}, nil
		}
		if isRiskySupportSummary(event.Summary) {
			return PolicyDecision{Action: roles.ActionEscalate, Reason: "needs human review"}, nil
		}
		if snap.ReplyAttemptCount >= e.maxAutoReplyCount {
			return PolicyDecision{Action: roles.ActionEscalate, Reason: "auto-reply budget exhausted"}, nil
		}
		return e.callExecutor(ctx, snap, event)

	case events.EventTypeQuestion:
		if isRepeatedSupportEvent(snap, event) {
			return PolicyDecision{Action: roles.ActionNoop}, nil
		}
		if isRiskySupportSummary(event.Summary) {
			return PolicyDecision{Action: roles.ActionEscalate, Reason: "needs human review"}, nil
		}
		if snap.ReplyAttemptCount >= e.maxAutoReplyCount {
			return PolicyDecision{Action: roles.ActionEscalate, Reason: "auto-reply budget exhausted"}, nil
		}
		return e.callExecutor(ctx, snap, event)

	default:
		return PolicyDecision{Action: roles.ActionNoop}, nil
	}
}

func (e *Engine) callExecutor(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (PolicyDecision, error) {
	d, err := e.executor.Decide(ctx, snap, event)
	if err != nil {
		return PolicyDecision{Action: roles.ActionNoop}, fmt.Errorf("executor: %w", err)
	}
	return PolicyDecision{Action: d.Action, Message: d.Message, Reason: d.Reason}, nil
}

func isRepeatedSupportEvent(snap state.Snapshot, event events.NormalizedEvent) bool {
	summary := strings.TrimSpace(event.Summary)
	if summary == "" || len(snap.RecentEvents) < 2 {
		return false
	}

	switch event.EventType {
	case events.EventTypeBlocked:
		if strings.TrimSpace(snap.LastBlocker) != summary {
			return false
		}
	case events.EventTypeQuestion:
		if strings.TrimSpace(snap.LastQuestion) != summary {
			return false
		}
	default:
		return false
	}

	last := strings.TrimSpace(snap.RecentEvents[len(snap.RecentEvents)-1])
	prev := strings.TrimSpace(snap.RecentEvents[len(snap.RecentEvents)-2])
	return last == summary && prev == summary
}

func isRiskySupportSummary(summary string) bool {
	lower := strings.ToLower(strings.TrimSpace(summary))
	if lower == "" {
		return false
	}

	riskyTokens := []string{
		"token",
		"secret",
		"password",
		"credential",
		"api key",
		"production",
		"prod",
		"vps",
		"ssh",
		"network",
		"internet",
		"delete",
		"destroy",
		"rm -rf",
	}
	for _, token := range riskyTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
