package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type botClient interface {
	GetUpdates(ctx context.Context, offset int64) ([]telegram.Update, error)
	SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error
	CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error)
	DeleteForumTopic(ctx context.Context, chatID int64, threadID int) error
}

type codexSessionRunner interface {
	Start(ctx context.Context, prompt string) (codexprovider.SessionResult, error)
	Resume(ctx context.Context, providerSessionID, prompt string) (codexprovider.SessionResult, error)
}

type policyEngine interface {
	Evaluate(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (policy.PolicyDecision, error)
}

type Runtime struct {
	cfg              config.Config
	store            *state.Store
	bot              botClient
	codex            codexSessionRunner
	policy           policyEngine
	auth             *telegram.Authorizer
	sessionsByThread map[int]state.SessionID
	mu               sync.RWMutex
}

func NewRuntime(cfg config.Config, store *state.Store, bot botClient, codex codexSessionRunner, policy policyEngine) *Runtime {
	return &Runtime{
		cfg:              cfg,
		store:            store,
		bot:              bot,
		codex:            codex,
		policy:           policy,
		auth:             telegram.NewAuthorizer(cfg.Telegram.AdminIDs, cfg.Telegram.OperatorIDs),
		sessionsByThread: make(map[int]state.SessionID),
	}
}

func (r *Runtime) HandleUpdate(ctx context.Context, update telegram.Update) error {
	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	threadID := update.Message.ThreadID
	if threadID == 0 {
		threadID = 1
	}

	if threadID == 1 {
		return r.handleGeneralTopic(ctx, update.Message.ChatID, update.Message.UserID, text)
	}
	return r.handleSessionTopic(ctx, update.Message.ChatID, threadID, update.Message.UserID, text)
}

func (r *Runtime) handleGeneralTopic(ctx context.Context, chatID, userID int64, text string) error {
	if !r.auth.CanSendCommand(userID) {
		return nil
	}

	text = normalizeGeneralCommand(text)

	switch {
	case text == "status":
		msg := fmt.Sprintf("status: ok (%d sessions)", r.sessionCount())
		return r.bot.SendMessage(ctx, chatID, nil, msg)

	case strings.HasPrefix(text, "cleanup "):
		return r.cleanupTopics(ctx, chatID, strings.TrimSpace(strings.TrimPrefix(text, "cleanup ")))

	case strings.HasPrefix(text, "start "):
		goal := strings.TrimSpace(strings.TrimPrefix(text, "start "))
		if goal == "" {
			return nil
		}
		if err := r.startSession(ctx, chatID, userID, goal); err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "start failed: "+err.Error())
		}
		return nil

	default:
		return nil
	}
}

func (r *Runtime) cleanupTopics(ctx context.Context, chatID int64, raw string) error {
	threadIDs, sessions, err := r.cleanupTargets(raw)
	if err != nil {
		return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
	}

	deleted := 0
	for _, threadID := range threadIDs {
		if err := r.bot.DeleteForumTopic(ctx, chatID, threadID); err != nil {
			if !isTopicGoneError(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		}

		r.mu.Lock()
		delete(r.sessionsByThread, threadID)
		r.mu.Unlock()
		deleted++
	}

	for _, session := range sessions {
		if session.SessionTopicID > 1 {
			r.mu.Lock()
			delete(r.sessionsByThread, int(session.SessionTopicID))
			r.mu.Unlock()
		}
	}

	for _, session := range sessions {
		if session.ActiveRunID != "" {
			if err := r.store.DeleteRun(session.ActiveRunID); err != nil && !isNotFound(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		}
		if err := r.store.DeleteSession(session.ID); err != nil && !isNotFound(err) {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
		}
	}

	return r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("cleanup: deleted %d topic(s)", deleted))
}

func (r *Runtime) startSession(ctx context.Context, chatID, userID int64, goal string) error {
	threadID, err := r.bot.CreateForumTopic(ctx, chatID, goal)
	if err != nil {
		return fmt.Errorf("create forum topic: %w", err)
	}

	if err := r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session started: %s", goal)); err != nil {
		return fmt.Errorf("send general start notice: %w", err)
	}

	sessionID := state.SessionID(makeID("ses"))
	runID := state.RunID(makeID("run"))
	now := time.Now().UTC()

	session := state.Session{
		ID:               sessionID,
		ActiveRunID:      runID,
		Provider:         "codex",
		GeneralTopicID:   1,
		SessionTopicID:   int64(threadID),
		Status:           state.SessionStatusRunning,
		Phase:            state.SessionPhasePlanning,
		LastGoal:         goal,
		EscalationState:  state.EscalationStateNone,
		OwnerUserID:      userID,
		LastHumanActorID: userID,
	}
	run := state.Run{
		ID:                runID,
		SessionID:         sessionID,
		Provider:          "codex",
		ProviderSessionID: "",
		Status:            state.RunStatusActive,
		StartedAt:         now,
		UpdatedAt:         now,
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

	if err := r.bot.SendMessage(ctx, chatID, &threadID, "Session starting: "+goal); err != nil {
		return err
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
	run, err := r.store.LoadRun(session.ActiveRunID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	if strings.TrimSpace(run.ProviderSessionID) == "" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Session is still starting; try again shortly.")
	}

	result, err := r.codex.Resume(ctx, run.ProviderSessionID, text)
	if err != nil {
		return fmt.Errorf("resume codex session: %w", err)
	}

	newRunID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	newRun := state.Run{
		ID:                newRunID,
		SessionID:         session.ID,
		Provider:          "codex",
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

func makeID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func normalizeGeneralCommand(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	if strings.HasPrefix(strings.ToLower(text), "clean up ") {
		text = "cleanup " + strings.TrimSpace(text[len("clean up "):])
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}

	command := fields[0]
	if !strings.HasPrefix(command, "/") {
		return text
	}

	command = strings.TrimPrefix(command, "/")
	if idx := strings.Index(command, "@"); idx >= 0 {
		command = command[:idx]
	}

	if len(fields) == 1 {
		return command
	}
	return command + " " + strings.Join(fields[1:], " ")
}

func parseThreadIDs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	threadIDs := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		threadID, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid topic id %q", value)
		}
		threadIDs = append(threadIDs, threadID)
	}

	if len(threadIDs) == 0 {
		return nil, fmt.Errorf("at least one topic id is required")
	}
	return threadIDs, nil
}

func (r *Runtime) cleanupTargets(raw string) ([]int, []state.Session, error) {
	sessions, err := r.store.ListSessions()
	if err != nil {
		return nil, nil, fmt.Errorf("list sessions: %w", err)
	}

	raw = strings.TrimSpace(raw)
	if raw == "all" {
		seen := make(map[int]struct{}, len(sessions))
		threadIDs := make([]int, 0, len(sessions))
		cleanupSessions := make([]state.Session, 0, len(sessions))
		for _, session := range sessions {
			if session.SessionTopicID <= 1 {
				continue
			}

			threadID := int(session.SessionTopicID)
			if _, ok := seen[threadID]; !ok {
				seen[threadID] = struct{}{}
				threadIDs = append(threadIDs, threadID)
			}
			cleanupSessions = append(cleanupSessions, session)
		}
		return threadIDs, cleanupSessions, nil
	}

	threadIDs, err := parseThreadIDs(raw)
	if err != nil {
		return nil, nil, err
	}
	threadIDs = dedupeThreadIDs(threadIDs)

	byThread := make(map[int][]state.Session, len(sessions))
	for _, session := range sessions {
		byThread[int(session.SessionTopicID)] = append(byThread[int(session.SessionTopicID)], session)
	}

	cleanupSessions := make([]state.Session, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		cleanupSessions = append(cleanupSessions, byThread[threadID]...)
	}
	return threadIDs, cleanupSessions, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, state.ErrNotFound)
}

func isTopicGoneError(err error) bool {
	return strings.Contains(err.Error(), "TOPIC_ID_INVALID")
}

func dedupeThreadIDs(threadIDs []int) []int {
	seen := make(map[int]struct{}, len(threadIDs))
	deduped := make([]int, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		if _, ok := seen[threadID]; ok {
			continue
		}
		seen[threadID] = struct{}{}
		deduped = append(deduped, threadID)
	}
	return deduped
}

func (r *Runtime) finishStart(ctx context.Context, chatID int64, threadID int, session state.Session, run state.Run, goal string) {
	result, err := r.codex.Start(ctx, goal)
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

func (r *Runtime) sessionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessionsByThread)
}

func (r *Runtime) sessionForThread(threadID int) (state.SessionID, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.sessionsByThread[threadID]
	return id, ok
}

func (r *Runtime) maybeAutoReply(ctx context.Context, chatID int64, threadID int, session state.Session, run state.Run, message string) error {
	if r.policy == nil || strings.TrimSpace(message) == "" {
		return nil
	}

	rawType := codexprovider.ClassifyText(message)
	if rawType == "" {
		return nil
	}

	event, err := events.Normalize(events.Observation{
		SessionID:    string(session.ID),
		RunID:        string(run.ID),
		Provider:     events.ProviderCodex,
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
		return fmt.Errorf("send agent reply note: %w", err)
	}

	result, err := r.codex.Resume(ctx, run.ProviderSessionID, decision.Message)
	if err != nil {
		return fmt.Errorf("resume codex after support reply: %w", err)
	}

	newRunID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	if err := r.store.SaveRun(state.Run{
		ID:                newRunID,
		SessionID:         session.ID,
		Provider:          "codex",
		ProviderSessionID: result.ProviderSessionID,
		Status:            state.RunStatusExited,
		StartedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		return fmt.Errorf("save support follow-up run: %w", err)
	}

	session.ActiveRunID = newRunID
	session.LastRoleUsed = "support"
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save support follow-up session: %w", err)
	}

	if strings.TrimSpace(result.Message) == "" {
		return nil
	}
	return r.bot.SendMessage(ctx, chatID, &threadID, result.Message)
}
