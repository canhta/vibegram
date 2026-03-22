package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/vibegram/internal/events"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
)

func (r *Runtime) launchDraftSession(ctx context.Context, chatID, userID int64, draft generalDraft) error {
	goal := draft.launchPrompt()
	threadID, err := r.bot.CreateForumTopic(ctx, chatID, topicNameForDraft(draft))
	if err != nil {
		return fmt.Errorf("create forum topic: %w", err)
	}

	sessionID := state.SessionID(makeID("ses"))
	runID := state.RunID(makeID("run"))
	now := time.Now().UTC()

	session := state.Session{
		ID:               sessionID,
		ActiveRunID:      runID,
		Provider:         draft.Provider,
		GeneralTopicID:   1,
		SessionTopicID:   int64(threadID),
		Status:           state.SessionStatusRunning,
		Phase:            state.SessionPhasePlanning,
		LastGoal:         goal,
		EscalationState:  state.EscalationStateNone,
		OwnerUserID:      userID,
		LastHumanActorID: userID,
		WorkRoot:         draft.WorkRoot,
	}
	run := state.Run{
		ID:        runID,
		SessionID: sessionID,
		Provider:  draft.Provider,
		Status:    state.RunStatusActive,
		StartedAt: now,
		UpdatedAt: now,
	}

	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := r.store.SaveRun(run); err != nil {
		return fmt.Errorf("save run: %w", err)
	}

	r.mu.Lock()
	r.sessionsByThread[threadID] = sessionID
	r.mu.Unlock()

	if err := r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session started: %s", topicNameForDraft(draft))); err != nil {
		return fmt.Errorf("send general start notice: %w", err)
	}
	if err := r.bot.SendMessage(ctx, chatID, &threadID, renderSessionStartSummary(draft)); err != nil {
		return fmt.Errorf("send session summary: %w", err)
	}

	go r.finishStart(ctx, chatID, threadID, session, run, goal)
	return nil
}

func (r *Runtime) handleSessionTopic(ctx context.Context, chatID int64, threadID int, userID int64, text string) error {
	if !r.auth.CanSendCommand(userID) {
		return nil
	}

	sessionID, ok := r.sessionForThread(threadID)
	if !ok {
		return nil
	}

	session, err := r.store.LoadSession(sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session.ActiveRunID == "" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "This session is not running right now.")
	}

	run, err := r.store.LoadRun(session.ActiveRunID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	if strings.TrimSpace(run.ProviderSessionID) == "" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Session is still starting; try again shortly.")
	}

	runner := r.runnerForProvider(session.Provider)
	if runner == nil {
		return fmt.Errorf("unknown provider %q", session.Provider)
	}

	result, err := runner.Resume(ctx, session.WorkRoot, run.ProviderSessionID, text)
	if err != nil {
		return fmt.Errorf("resume provider session: %w", err)
	}

	newRunID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	newRun := state.Run{
		ID:                newRunID,
		SessionID:         session.ID,
		Provider:          session.Provider,
		ProviderSessionID: result.ProviderSessionID,
		Status:            state.RunStatusExited,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := r.store.SaveRun(newRun); err != nil {
		return fmt.Errorf("save resumed run: %w", err)
	}

	session.ActiveRunID = newRunID
	session.LastHumanActorID = userID
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save resumed session: %w", err)
	}

	return r.bot.SendMessage(ctx, chatID, &threadID, result.Message)
}

func (r *Runtime) finishStart(ctx context.Context, chatID int64, threadID int, session state.Session, run state.Run, goal string) {
	runner := r.runnerForProvider(session.Provider)
	if runner == nil {
		_ = r.bot.SendMessage(ctx, chatID, &threadID, "start failed: unknown provider "+session.Provider)
		return
	}

	result, err := runner.Start(ctx, session.WorkRoot, goal)
	if err != nil {
		_ = r.bot.SendMessage(ctx, chatID, &threadID, "start failed: "+err.Error())
		return
	}

	run.ProviderSessionID = result.ProviderSessionID
	run.Status = state.RunStatusExited
	run.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveRun(run); err != nil {
		_ = r.bot.SendMessage(ctx, chatID, &threadID, "start failed: save run: "+err.Error())
		return
	}

	if strings.TrimSpace(result.Message) != "" {
		if err := r.bot.SendMessage(ctx, chatID, &threadID, result.Message); err != nil {
			_ = r.bot.SendMessage(ctx, chatID, &threadID, "start failed: "+err.Error())
			return
		}
	}

	_ = r.maybeAutoReply(ctx, chatID, threadID, session, run, result.Message)
}

func (r *Runtime) runnerForProvider(provider string) sessionRunner {
	switch provider {
	case "codex":
		return r.codex
	case "claude":
		return r.claude
	default:
		return nil
	}
}

func (r *Runtime) maybeAutoReply(ctx context.Context, chatID int64, threadID int, session state.Session, run state.Run, message string) error {
	if r.policy == nil || strings.TrimSpace(message) == "" {
		return nil
	}

	runner := r.runnerForProvider(session.Provider)
	if runner == nil {
		return nil
	}

	rawType := codexprovider.ClassifyText(message)
	if rawType == "" {
		return nil
	}

	event, err := events.Normalize(events.Observation{
		SessionID:    string(session.ID),
		RunID:        string(run.ID),
		Provider:     eventProviderForSession(session.Provider),
		Source:       events.SourceTranscript,
		RawType:      rawType,
		RawTimestamp: time.Now().UTC(),
		Summary:      message,
		ProviderID:   run.ProviderSessionID,
	})
	if err != nil {
		return fmt.Errorf("normalize support event: %w", err)
	}

	snap, err := r.store.LoadSnapshot(string(session.ID))
	if err != nil {
		if err == state.ErrNotFound || strings.Contains(err.Error(), state.ErrNotFound.Error()) {
			snap = state.Snapshot{
				SessionID: string(session.ID),
				Phase:     string(session.Phase),
				Status:    string(session.Status),
			}
		} else {
			return fmt.Errorf("load snapshot: %w", err)
		}
	}
	snap.Apply(event)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	decision, err := r.policy.Evaluate(ctx, snap, event)
	if err != nil {
		return fmt.Errorf("evaluate support policy: %w", err)
	}
	if decision.Action != roles.ActionReply || strings.TrimSpace(decision.Message) == "" {
		return nil
	}

	if err := r.bot.SendMessage(ctx, chatID, &threadID, "Agent reply: "+decision.Message); err != nil {
		return fmt.Errorf("send auto-reply note: %w", err)
	}
	replyResult, err := runner.Resume(ctx, session.WorkRoot, run.ProviderSessionID, decision.Message)
	if err != nil {
		return fmt.Errorf("resume after support reply: %w", err)
	}

	replyRunID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	replyRun := state.Run{
		ID:                replyRunID,
		SessionID:         session.ID,
		Provider:          session.Provider,
		ProviderSessionID: replyResult.ProviderSessionID,
		Status:            state.RunStatusExited,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := r.store.SaveRun(replyRun); err != nil {
		return fmt.Errorf("save auto-reply run: %w", err)
	}

	session.ActiveRunID = replyRunID
	session.LastRoleUsed = "support"
	session.ReplyAttemptCount++
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save auto-reply session: %w", err)
	}

	if strings.TrimSpace(replyResult.Message) == "" {
		return nil
	}
	return r.bot.SendMessage(ctx, chatID, &threadID, replyResult.Message)
}

func topicNameForDraft(draft generalDraft) string {
	title := strings.TrimSpace(draft.Task)
	if title == "" {
		title = draft.launchPrompt()
	}
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > 64 {
		title = title[:64]
	}
	if title == "" {
		return "new session"
	}
	return title
}

func renderSessionStartSummary(draft generalDraft) string {
	return strings.Join([]string{
		fmt.Sprintf("Agent: %s", draft.Provider),
		fmt.Sprintf("Folder: %s", draft.WorkRoot),
		fmt.Sprintf("Goal: %s", draft.launchPrompt()),
		"Launching...",
	}, "\n")
}

func eventProviderForSession(provider string) events.Provider {
	switch provider {
	case "claude":
		return events.ProviderClaude
	case "codex":
		return events.ProviderCodex
	default:
		return events.ProviderCodex
	}
}
