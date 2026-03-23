package policy

import (
	"context"
	"fmt"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
)

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
	executor RoleExecutor
}

func NewEngine(executor RoleExecutor) *Engine {
	return &Engine{executor: executor}
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
		if snap.ReplyAttemptCount >= 3 {
			return PolicyDecision{Action: roles.ActionEscalate, Reason: "reply attempt ceiling reached"}, nil
		}
		return e.callExecutor(ctx, snap, event)

	case events.EventTypeQuestion:
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
