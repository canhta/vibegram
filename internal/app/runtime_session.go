package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/canhta/vibegram/internal/events"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
)

func (r *Runtime) launchDraftSession(ctx context.Context, chatID, userID int64, draft generalDraft) error {
	goal := draft.launchPrompt()
	sessionID := state.SessionID(makeID("ses"))
	runID := state.RunID(makeID("run"))
	topicName := topicNameForDraft(draft, shortTopicCode(sessionID))
	threadID, err := r.bot.CreateForumTopic(ctx, chatID, topicName)
	if err != nil {
		return fmt.Errorf("create forum topic: %w", err)
	}
	now := time.Now().UTC()

	session := state.Session{
		ID:                sessionID,
		ActiveRunID:       runID,
		Provider:          draft.Provider,
		GeneralTopicID:    1,
		SessionTopicID:    int64(threadID),
		SessionTopicTitle: topicName,
		Status:            state.SessionStatusRunning,
		Phase:             state.SessionPhasePlanning,
		LastGoal:          goal,
		EscalationState:   state.EscalationStateNone,
		SupportState:      state.SupportStateIdle,
		OwnerUserID:       userID,
		LastHumanActorID:  userID,
		WorkRoot:          draft.WorkRoot,
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
	if err := r.upsertSessionHeaderCard(ctx, chatID, threadID, &session, true); err != nil {
		return fmt.Errorf("create session header card: %w", err)
	}

	r.mu.Lock()
	r.sessionsByThread[threadID] = sessionID
	r.mu.Unlock()

	if err := r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session started: %s", topicName)); err != nil {
		return fmt.Errorf("send general start notice: %w", err)
	}

	r.wg.Add(1)
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

	streamRun := state.Run{
		ID:                state.RunID(makeID("run")),
		SessionID:         session.ID,
		Provider:          session.Provider,
		ProviderSessionID: run.ProviderSessionID,
		Status:            state.RunStatusActive,
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	observer := newStreamObserver(r, ctx, chatID, threadID, &session, streamRun)

	var result codexprovider.SessionResult
	streamer, canStream := runner.(streamingSessionRunner)
	if canStream {
		result, err = streamer.ResumeStream(ctx, session.WorkRoot, run.ProviderSessionID, text, observer.OnLine)
	} else {
		result, err = runner.Resume(ctx, session.WorkRoot, run.ProviderSessionID, text)
	}
	if err != nil {
		session.LastHumanActorID = userID
		return r.recordResumeFailure(ctx, chatID, threadID, &session, run, err)
	}
	if err := observer.Err(); err != nil {
		return err
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

	return r.deliverSessionResult(ctx, chatID, threadID, &session, newRun, result, observer.deduper, true)
}

func (r *Runtime) finishStart(ctx context.Context, chatID int64, threadID int, session state.Session, run state.Run, goal string) {
	defer r.wg.Done()
	runner := r.runnerForProvider(session.Provider)
	if runner == nil {
		_ = r.recordStartFailure(ctx, chatID, threadID, &session, run, fmt.Errorf("unknown provider %s", session.Provider))
		return
	}

	observer := newStreamObserver(r, ctx, chatID, threadID, &session, run)

	var result codexprovider.SessionResult
	var err error
	streamer, canStream := runner.(streamingSessionRunner)
	if canStream {
		result, err = streamer.StartStream(ctx, session.WorkRoot, goal, observer.OnLine)
	} else {
		result, err = runner.Start(ctx, session.WorkRoot, goal)
	}
	if err != nil {
		_ = r.recordStartFailure(ctx, chatID, threadID, &session, run, err)
		return
	}
	if err := observer.Err(); err != nil {
		_ = r.recordStartFailure(ctx, chatID, threadID, &session, run, err)
		return
	}

	run.ProviderSessionID = result.ProviderSessionID
	run.Status = state.RunStatusExited
	run.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveRun(run); err != nil {
		_ = r.sendSessionMessage(ctx, chatID, threadID, &session, "start failed: save run: "+err.Error())
		return
	}

	if err := r.deliverSessionResult(ctx, chatID, threadID, &session, run, result, observer.deduper, true); err != nil {
		_ = r.sendSessionMessage(ctx, chatID, threadID, &session, "start failed: "+err.Error())
	}
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

func (r *Runtime) maybeAutoReply(ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run, message string) error {
	if strings.TrimSpace(message) == "" {
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
	if err := r.projectSessionEvent(ctx, chatID, threadID, session, event, r.policy != nil); err != nil {
		return err
	}
	return r.maybeAutoReplyForEvent(ctx, chatID, threadID, session, run, event)
}

func (r *Runtime) maybeAutoReplyForEvent(ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run, event events.NormalizedEvent) error {
	if r.policy == nil {
		return nil
	}

	runner := r.runnerForProvider(session.Provider)
	if runner == nil {
		return nil
	}

	snap, err := r.loadSessionSnapshot(*session)
	if err != nil {
		return err
	}

	decision, err := r.policy.Evaluate(ctx, snap, event)
	if err != nil {
		return fmt.Errorf("evaluate support policy: %w", err)
	}
	switch decision.Action {
	case roles.ActionReply:
		if strings.TrimSpace(decision.Message) == "" {
			return nil
		}
		if err := r.applySupportDecision(ctx, chatID, threadID, session, snap, state.SupportStateReplied, decision.Message, false); err != nil {
			return err
		}
		if err := r.sendSessionMessage(ctx, chatID, threadID, session, "Agent reply: "+decision.Message); err != nil {
			return fmt.Errorf("send auto-reply note: %w", err)
		}
		replyResult, err := runner.Resume(ctx, session.WorkRoot, run.ProviderSessionID, decision.Message)
		if err != nil {
			session.LastRoleUsed = "support"
			session.ReplyAttemptCount++
			return r.recordResumeFailure(ctx, chatID, threadID, session, run, err)
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
		snap.ReplyAttemptCount = session.ReplyAttemptCount
		if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
			return fmt.Errorf("save auto-reply snapshot: %w", err)
		}
		if err := r.store.SaveSession(*session); err != nil {
			return fmt.Errorf("save auto-reply session: %w", err)
		}
		return r.deliverSessionResult(ctx, chatID, threadID, session, replyRun, replyResult, nil, false)
	case roles.ActionEscalate:
		summary := strings.TrimSpace(decision.Reason)
		if summary == "" {
			summary = strings.TrimSpace(decision.Message)
		}
		return r.applySupportDecision(ctx, chatID, threadID, session, snap, state.SupportStateEscalated, summary, true)
	default:
		return nil
	}
}

func (r *Runtime) recordResumeFailure(ctx context.Context, chatID int64, threadID int, session *state.Session, previousRun state.Run, resumeErr error) error {
	now := time.Now().UTC()
	failedRun := state.Run{
		ID:                state.RunID(makeID("run")),
		SessionID:         session.ID,
		Provider:          session.Provider,
		ProviderSessionID: previousRun.ProviderSessionID,
		Status:            state.RunStatusFailed,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := r.store.SaveRun(failedRun); err != nil {
		return fmt.Errorf("save failed run: %w", err)
	}

	session.ActiveRunID = failedRun.ID
	session.Status = state.SessionStatusFailed
	session.LastBlocker = resumeErr.Error()
	session.EscalationState = state.EscalationStateNeeded
	snap, err := r.loadSessionSnapshot(*session)
	if err != nil {
		return err
	}
	snap.Status = string(session.Status)
	snap.LastBlocker = session.LastBlocker
	snap.ReplyAttemptCount = session.ReplyAttemptCount
	snap.ApplySupportDecision(state.SupportStateEscalated, resumeErr.Error(), true)
	session.ApplySnapshot(snap)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save failed snapshot: %w", err)
	}
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("save failed session: %w", err)
	}
	if err := r.upsertSessionHeaderCard(ctx, chatID, threadID, session, true); err != nil {
		return fmt.Errorf("update failed session header card: %w", err)
	}
	if err := r.maybeSendSupportAwareness(ctx, chatID, *session, state.SupportStateEscalated, resumeErr.Error()); err != nil {
		return fmt.Errorf("send resume escalation awareness: %w", err)
	}

	if err := r.sendSessionMessage(ctx, chatID, threadID, session, "resume failed: "+resumeErr.Error()); err != nil {
		return fmt.Errorf("send resume failure: %w", err)
	}
	return nil
}

func (r *Runtime) recordStartFailure(ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run, startErr error) error {
	run.Status = state.RunStatusFailed
	run.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveRun(run); err != nil {
		return fmt.Errorf("save failed start run: %w", err)
	}

	session.Status = state.SessionStatusFailed
	session.LastBlocker = startErr.Error()
	session.EscalationState = state.EscalationStateNeeded
	snap, err := r.loadSessionSnapshot(*session)
	if err != nil {
		return err
	}
	snap.Status = string(session.Status)
	snap.LastBlocker = session.LastBlocker
	snap.ApplySupportDecision(state.SupportStateEscalated, startErr.Error(), true)
	session.ApplySnapshot(snap)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save failed start snapshot: %w", err)
	}
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("save failed start session: %w", err)
	}
	if err := r.upsertSessionHeaderCard(ctx, chatID, threadID, session, true); err != nil {
		return fmt.Errorf("update failed start session header card: %w", err)
	}
	if err := r.maybeSendSupportAwareness(ctx, chatID, *session, state.SupportStateEscalated, startErr.Error()); err != nil {
		return fmt.Errorf("send start escalation awareness: %w", err)
	}
	if err := r.sendSessionMessage(ctx, chatID, threadID, session, "start failed: "+startErr.Error()); err != nil {
		return fmt.Errorf("send start failure: %w", err)
	}
	return nil
}

func topicNameForDraft(draft generalDraft, shortCode string) string {
	folder := strings.TrimSpace(filepath.Base(strings.TrimSpace(draft.WorkRoot)))
	if folder == "" || folder == "." || folder == string(filepath.Separator) {
		folder = "session"
	}
	provider := strings.TrimSpace(draft.Provider)
	if provider == "" {
		provider = "agent"
	}
	if shortCode == "" {
		shortCode = "0000"
	}
	suffix := strings.Join([]string{provider, shortCode}, " ")
	maxFolderLen := 64 - len(suffix) - 1
	if maxFolderLen < 1 {
		maxFolderLen = 1
	}
	if len(folder) > maxFolderLen {
		folder = folder[:maxFolderLen]
	}
	title := strings.Join([]string{folder, suffix}, " ")
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > 64 {
		title = title[:64]
	}
	return title
}

func shortTopicCode(sessionID state.SessionID) string {
	raw := string(sessionID)
	digits := make([]rune, 0, len(raw))
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			digits = append(digits, ch)
		}
	}
	if len(digits) == 0 {
		return "0000"
	}
	if len(digits) >= 4 {
		return string(digits[len(digits)-4:])
	}
	return fmt.Sprintf("%04s", string(digits))
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
