package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

func (r *Runtime) projectSessionEvent(ctx context.Context, chatID int64, threadID int, session *state.Session, event events.NormalizedEvent, policyAvailable bool) error {
	if session == nil {
		return nil
	}

	snap, err := r.loadSessionSnapshot(*session)
	if err != nil {
		return err
	}

	snap.Apply(event)
	switch event.EventType {
	case events.EventTypeBlocked, events.EventTypeQuestion:
		if !policyAvailable {
			snap.ApplySupportFallback(event)
		}
	case events.EventTypeApprovalNeeded, events.EventTypeFailed:
		snap.ApplySupportFallback(event)
	case events.EventTypeBlockerResolved, events.EventTypeDone:
		snap.ApplySupportFallback(event)
	}

	session.ApplySnapshot(snap)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return r.upsertSessionHeaderCard(ctx, chatID, threadID, session, true)
}

func (r *Runtime) applySupportDecision(ctx context.Context, chatID int64, threadID int, session *state.Session, snap state.Snapshot, supportState state.SupportState, summary string, humanActionNeeded bool) error {
	if session == nil {
		return nil
	}

	snap.ApplySupportDecision(supportState, summary, humanActionNeeded)
	session.ApplySnapshot(snap)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := r.upsertSessionHeaderCard(ctx, chatID, threadID, session, true); err != nil {
		return err
	}
	return r.maybeSendSupportAwareness(ctx, chatID, *session, supportState, summary)
}

func (r *Runtime) loadSessionSnapshot(session state.Session) (state.Snapshot, error) {
	snap, err := r.store.LoadSnapshot(string(session.ID))
	if err == nil {
		return snap, nil
	}
	if err != nil && !isNotFound(err) {
		return state.Snapshot{}, fmt.Errorf("load snapshot: %w", err)
	}

	return state.Snapshot{
		SessionID:              string(session.ID),
		Phase:                  string(session.Phase),
		Status:                 string(session.Status),
		LastBlocker:            session.LastBlocker,
		LastQuestion:           session.LastQuestion,
		RecentFilesSummary:     session.RecentFilesSummary,
		RecentTestsSummary:     session.RecentTestsSummary,
		RecentEvents:           append([]string(nil), session.RecentEvents...),
		ReplyAttemptCount:      session.ReplyAttemptCount,
		EscalationState:        string(session.EscalationState),
		SupportState:           string(session.SupportState),
		SupportDecisionSummary: session.SupportDecisionSummary,
		HumanActionNeeded:      session.HumanActionNeeded,
	}, nil
}

func (r *Runtime) upsertSessionHeaderCard(ctx context.Context, chatID int64, threadID int, session *state.Session, createIfMissing bool) error {
	if session == nil {
		return nil
	}

	text := renderSessionHeaderCard(*session)
	markup := telegram.InlineKeyboardMarkup{}
	if session.SessionHeaderMessageID == 0 {
		if !createIfMissing {
			return r.refreshGeneralControlCardIfPresent(ctx, chatID)
		}
		messageID, err := r.bot.SendMessageCard(ctx, chatID, &threadID, text, markup)
		if err != nil {
			if isTopicGoneError(err) {
				return r.retireMissingSessionTopic(ctx, chatID, session, err)
			}
			return fmt.Errorf("send session header card: %w", err)
		}
		session.SessionHeaderMessageID = messageID
		if err := r.store.SaveSession(*session); err != nil {
			return fmt.Errorf("save session header id: %w", err)
		}
		return r.refreshGeneralControlCardIfPresent(ctx, chatID)
	}

	if err := r.bot.EditMessageCard(ctx, chatID, session.SessionHeaderMessageID, text, markup); err != nil {
		if isTopicGoneError(err) {
			return r.retireMissingSessionTopic(ctx, chatID, session, err)
		}
		return fmt.Errorf("edit session header card: %w", err)
	}
	return r.refreshGeneralControlCardIfPresent(ctx, chatID)
}

func renderSessionHeaderCard(session state.Session) string {
	lines := []string{
		fmt.Sprintf("Agent: %s", telegram.TruncateText(sessionOrUnknown(session.Provider, "agent"), 32)),
		fmt.Sprintf("Folder: %s", telegram.TruncateText(sessionFolderLabel(session.WorkRoot), 96)),
		fmt.Sprintf("Goal: %s", telegram.TruncateText(sessionOrUnknown(session.LastGoal, "none"), 320)),
		fmt.Sprintf("State: %s", telegram.TruncateText(sessionStateLabel(session), 64)),
		fmt.Sprintf("Support: %s", telegram.TruncateText(supportStateLabel(session.SupportState), 32)),
		fmt.Sprintf("Support decision: %s", telegram.TruncateText(sessionOrUnknown(session.SupportDecisionSummary, "none"), 320)),
		fmt.Sprintf("Human action: %s", yesNo(session.HumanActionNeeded)),
	}
	return telegram.ClampMessageText(strings.Join(lines, "\n"))
}

func sessionStateLabel(session state.Session) string {
	status := strings.TrimSpace(string(session.Status))
	phase := strings.TrimSpace(string(session.Phase))
	if status == "" {
		status = "unknown"
	}
	if phase == "" {
		return status
	}
	return status + " / " + phase
}

func sessionFolderLabel(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "unknown"
	}

	label := strings.TrimSpace(filepath.Base(path))
	if label == "" || label == "." || label == string(filepath.Separator) {
		return path
	}
	return label
}

func supportStateLabel(supportState state.SupportState) string {
	label := strings.TrimSpace(string(supportState))
	if label == "" {
		return string(state.SupportStateIdle)
	}
	return strings.ReplaceAll(label, "_", " ")
}

func sessionOrUnknown(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func (r *Runtime) maybeSendSupportAwareness(ctx context.Context, chatID int64, session state.Session, supportState state.SupportState, summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}

	title := strings.TrimSpace(session.SessionTopicTitle)
	if title == "" {
		title = strings.TrimSpace(derivedTopicTitle(session))
	}
	if title == "" {
		title = string(session.ID)
	}

	var prefix string
	switch supportState {
	case state.SupportStateReplied:
		prefix = "Support replied in "
	case state.SupportStateEscalated:
		prefix = "Support escalated in "
	default:
		return nil
	}

	return r.bot.SendMessage(ctx, chatID, nil, prefix+title+": "+summary)
}
