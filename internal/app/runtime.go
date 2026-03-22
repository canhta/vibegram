package app

import (
	"context"
	"fmt"
	"strings"
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

	switch {
	case text == "status":
		msg := fmt.Sprintf("status: ok (%d sessions)", len(r.sessionsByThread))
		return r.bot.SendMessage(ctx, chatID, nil, msg)

	case strings.HasPrefix(text, "start "):
		goal := strings.TrimSpace(strings.TrimPrefix(text, "start "))
		if goal == "" {
			return nil
		}
		return r.startSession(ctx, chatID, userID, goal)

	default:
		return nil
	}
}

func (r *Runtime) startSession(ctx context.Context, chatID, userID int64, goal string) error {
	threadID, err := r.bot.CreateForumTopic(ctx, chatID, goal)
	if err != nil {
		return fmt.Errorf("create forum topic: %w", err)
	}

	if err := r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session started: %s", goal)); err != nil {
		return fmt.Errorf("send general start notice: %w", err)
	}

	result, err := r.codex.Start(ctx, goal)
	if err != nil {
		return fmt.Errorf("start codex session: %w", err)
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
		ProviderSessionID: result.ProviderSessionID,
		Status:            state.RunStatusExited,
		StartedAt:         now,
		UpdatedAt:         now,
	}

	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := r.store.SaveRun(run); err != nil {
		return fmt.Errorf("save run: %w", err)
	}

	r.sessionsByThread[threadID] = sessionID

	if err := r.bot.SendMessage(ctx, chatID, &threadID, result.Message); err != nil {
		return err
	}

	return r.maybeAutoReply(ctx, chatID, threadID, session, run, result.Message)
}

func (r *Runtime) handleSessionTopic(ctx context.Context, chatID int64, threadID int, userID int64, text string) error {
	if !r.auth.CanSendCommand(userID) {
		return nil
	}

	sessionID, ok := r.sessionsByThread[threadID]
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
