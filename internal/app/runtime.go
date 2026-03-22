package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	SendMessageCard(ctx context.Context, chatID int64, threadID *int, text string, markup telegram.InlineKeyboardMarkup) (int, error)
	EditMessageCard(ctx context.Context, chatID int64, messageID int, text string, markup telegram.InlineKeyboardMarkup) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
	CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error)
	DeleteForumTopic(ctx context.Context, chatID int64, threadID int) error
}

type sessionRunner interface {
	Start(ctx context.Context, workDir, prompt string) (codexprovider.SessionResult, error)
	Resume(ctx context.Context, workDir, providerSessionID, prompt string) (codexprovider.SessionResult, error)
}

type policyEngine interface {
	Evaluate(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (policy.PolicyDecision, error)
}

type supportResponder interface {
	Reply(ctx context.Context, text string) (string, error)
}

type Runtime struct {
	cfg              config.Config
	store            *state.Store
	bot              botClient
	codex            sessionRunner
	opencode         sessionRunner
	policy           policyEngine
	support          supportResponder
	auth             *telegram.Authorizer
	sessionsByThread map[int]state.SessionID
	mu               sync.RWMutex
}

func NewRuntime(cfg config.Config, store *state.Store, bot botClient, codex sessionRunner, opencode sessionRunner, policy policyEngine, support supportResponder) *Runtime {
	return &Runtime{
		cfg:              cfg,
		store:            store,
		bot:              bot,
		codex:            codex,
		opencode:         opencode,
		policy:           policy,
		support:          support,
		auth:             telegram.NewAuthorizer(cfg.Telegram.AdminIDs, cfg.Telegram.OperatorIDs),
		sessionsByThread: make(map[int]state.SessionID),
	}
}

func (r *Runtime) HandleUpdate(ctx context.Context, update telegram.Update) error {
	if update.CallbackQuery != nil {
		return r.handleCallback(ctx, *update.CallbackQuery)
	}

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

	if !strings.HasPrefix(strings.TrimSpace(text), "/") {
		if r.support == nil {
			return nil
		}
		reply, err := r.support.Reply(ctx, text)
		if err != nil {
			return fmt.Errorf("support reply: %w", err)
		}
		if strings.TrimSpace(reply) == "" {
			return nil
		}
		return r.bot.SendMessage(ctx, chatID, nil, reply)
	}

	text = normalizeGeneralCommand(text)

	switch {
	case text == "status":
		msg := fmt.Sprintf("status: ok (%d sessions)", r.sessionCount())
		return r.bot.SendMessage(ctx, chatID, nil, msg)

	case text == "cleanup":
		return r.bot.SendMessage(ctx, chatID, nil, "Usage: /cleanup <topic_id[,topic_id]> or /cleanup all")

	case strings.HasPrefix(text, "cleanup "):
		return r.cleanupTopics(ctx, chatID, strings.TrimSpace(strings.TrimPrefix(text, "cleanup ")))

	case text == "start":
		return r.bot.SendMessage(ctx, chatID, nil, "Usage: /start <goal>")

	case strings.HasPrefix(text, "start "):
		goal := strings.TrimSpace(strings.TrimPrefix(text, "start "))
		if goal == "" {
			return r.bot.SendMessage(ctx, chatID, nil, "Usage: /start <goal>")
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

	sessionID := state.SessionID(makeID("ses"))
	session := state.Session{
		ID:               sessionID,
		GeneralTopicID:   1,
		SessionTopicID:   int64(threadID),
		Status:           state.SessionStatusRunning,
		Phase:            state.SessionPhaseWaiting,
		LastGoal:         goal,
		EscalationState:  state.EscalationStateNone,
		OwnerUserID:      userID,
		LastHumanActorID: userID,
	}

	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	if err := r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session created: %s", goal)); err != nil {
		return fmt.Errorf("send general start notice: %w", err)
	}

	r.mu.Lock()
	r.sessionsByThread[threadID] = sessionID
	r.mu.Unlock()

	setupText, setupMarkup := r.renderSetupCard(session)
	messageID, err := r.bot.SendMessageCard(ctx, chatID, &threadID, setupText, setupMarkup)
	if err != nil {
		return fmt.Errorf("send setup card: %w", err)
	}
	session.SetupMessageID = messageID
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save setup session: %w", err)
	}

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
		return r.handleSetupTopicInput(ctx, chatID, threadID, session, text)
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

func makeID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func normalizeGeneralCommand(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
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

func (r *Runtime) handleCallback(ctx context.Context, callback telegram.CallbackQuery) error {
	if !r.auth.CanSendCommand(callback.FromUserID) {
		return nil
	}

	threadID := callback.Message.ThreadID
	if threadID == 0 {
		threadID = 1
	}
	if threadID == 1 {
		return r.bot.AnswerCallback(ctx, callback.ID, "")
	}

	sessionID, ok := r.sessionForThread(threadID)
	if !ok {
		return r.bot.AnswerCallback(ctx, callback.ID, "Session not found")
	}

	session, err := r.store.LoadSession(sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session.ActiveRunID != "" {
		return r.bot.AnswerCallback(ctx, callback.ID, "Session already launched")
	}

	switch callback.Data {
	case "set:prov:codex":
		session.Provider = "codex"
		session.SetupAwaiting = ""
	case "set:prov:opencode":
		session.Provider = "opencode"
		session.SetupAwaiting = ""
	case "set:dir:repo":
		session.WorkRoot = r.cfg.Runtime.WorkRoot
		session.SetupAwaiting = ""
	case "set:dir:desktop":
		session.WorkRoot = filepath.Join(userHomeDir(), "Desktop")
		session.SetupAwaiting = ""
	case "set:dir:projects":
		session.WorkRoot = filepath.Join(userHomeDir(), "Projects")
		session.SetupAwaiting = ""
	case "set:dir:custom":
		session.SetupAwaiting = "custom_path"
	case "set:launch":
		if err := r.bot.AnswerCallback(ctx, callback.ID, "Launching"); err != nil {
			return err
		}
		return r.launchSession(ctx, callback.Message.ChatID, threadID, session)
	default:
		return r.bot.AnswerCallback(ctx, callback.ID, "")
	}

	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := r.editSetupCard(ctx, callback.Message.ChatID, session); err != nil {
		return err
	}
	return r.bot.AnswerCallback(ctx, callback.ID, "")
}

func (r *Runtime) handleSetupTopicInput(ctx context.Context, chatID int64, threadID int, session state.Session, text string) error {
	if session.SetupAwaiting != "custom_path" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Finish setup with the buttons above before sending session messages.")
	}

	workDir, err := resolveWorkDir(text)
	if err != nil {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Invalid path: "+err.Error())
	}
	session.WorkRoot = workDir
	session.SetupAwaiting = ""
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return r.editSetupCard(ctx, chatID, session)
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

	_ = r.editSetupCard(ctx, chatID, session)
	_ = r.maybeAutoReply(ctx, chatID, threadID, session, run, result.Message)
}

func (r *Runtime) launchSession(ctx context.Context, chatID int64, threadID int, session state.Session) error {
	if strings.TrimSpace(session.Provider) == "" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Launch blocked: choose a provider first.")
	}
	if strings.TrimSpace(session.WorkRoot) == "" {
		return r.bot.SendMessage(ctx, chatID, &threadID, "Launch blocked: choose a directory first.")
	}

	runID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	run := state.Run{
		ID:        runID,
		SessionID: session.ID,
		Provider:  session.Provider,
		Status:    state.RunStatusActive,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := r.store.SaveRun(run); err != nil {
		return fmt.Errorf("save run: %w", err)
	}

	session.ActiveRunID = runID
	session.Phase = state.SessionPhasePlanning
	if err := r.store.SaveSession(session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := r.bot.EditMessageCard(ctx, chatID, session.SetupMessageID, r.renderLaunchingCard(session), telegram.InlineKeyboardMarkup{}); err != nil {
		return fmt.Errorf("edit launching card: %w", err)
	}

	go r.finishStart(ctx, chatID, threadID, session, run, session.LastGoal)
	return nil
}

func (r *Runtime) editSetupCard(ctx context.Context, chatID int64, session state.Session) error {
	if session.SetupMessageID == 0 {
		return nil
	}
	text, markup := r.renderSetupCard(session)
	return r.bot.EditMessageCard(ctx, chatID, session.SetupMessageID, text, markup)
}

func (r *Runtime) renderSetupCard(session state.Session) (string, telegram.InlineKeyboardMarkup) {
	provider := "not selected"
	if session.Provider != "" {
		provider = session.Provider
	}

	directory := "not selected"
	if session.WorkRoot != "" {
		directory = session.WorkRoot
	}

	lines := []string{
		fmt.Sprintf("Setup: %s", session.LastGoal),
		fmt.Sprintf("Provider: %s", provider),
		fmt.Sprintf("Directory: %s", directory),
	}
	if session.ActiveRunID != "" {
		lines = append(lines, "Status: running in this topic.")
	} else if session.SetupAwaiting == "custom_path" {
		lines = append(lines, "Next step: send the custom path as your next message in this topic.")
	} else if session.Provider == "" || session.WorkRoot == "" {
		lines = append(lines, "Next step: choose a provider and directory, then launch.")
	} else {
		lines = append(lines, "Ready: press Launch to start the session.")
	}

	rows := [][]telegram.InlineKeyboardButton{}
	providerRow := []telegram.InlineKeyboardButton{}
	if strings.TrimSpace(r.cfg.Providers.CodexCommand) != "" {
		providerRow = append(providerRow, telegram.InlineKeyboardButton{Text: providerLabel("codex", session.Provider), CallbackData: "set:prov:codex"})
	}
	if strings.TrimSpace(r.cfg.Providers.OpenCodeCommand) != "" {
		providerRow = append(providerRow, telegram.InlineKeyboardButton{Text: providerLabel("opencode", session.Provider), CallbackData: "set:prov:opencode"})
	}
	if len(providerRow) > 0 {
		rows = append(rows, providerRow)
	}

	rows = append(rows,
		[]telegram.InlineKeyboardButton{
			{Text: dirLabel("repo", session.WorkRoot, r.cfg.Runtime.WorkRoot), CallbackData: "set:dir:repo"},
			{Text: dirLabel("desktop", session.WorkRoot, filepath.Join(userHomeDir(), "Desktop")), CallbackData: "set:dir:desktop"},
		},
		[]telegram.InlineKeyboardButton{
			{Text: dirLabel("projects", session.WorkRoot, filepath.Join(userHomeDir(), "Projects")), CallbackData: "set:dir:projects"},
			{Text: "Custom Path", CallbackData: "set:dir:custom"},
		},
	)

	if session.ActiveRunID == "" && session.Provider != "" && session.WorkRoot != "" {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Launch", CallbackData: "set:launch"}})
	}

	return strings.Join(lines, "\n"), telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (r *Runtime) renderLaunchingCard(session state.Session) string {
	return strings.Join([]string{
		fmt.Sprintf("Setup: %s", session.LastGoal),
		fmt.Sprintf("Provider: %s", session.Provider),
		fmt.Sprintf("Directory: %s", session.WorkRoot),
		"Launching...",
	}, "\n")
}

func (r *Runtime) runnerForProvider(provider string) sessionRunner {
	switch provider {
	case "codex":
		return r.codex
	case "opencode":
		return r.opencode
	default:
		return nil
	}
}

func providerLabel(name, selected string) string {
	if name == selected {
		return name + " selected"
	}
	return name
}

func dirLabel(label, selected, target string) string {
	if selected == target {
		return label + " selected"
	}
	return label
}

func resolveWorkDir(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(path, "~") {
		path = filepath.Join(userHomeDir(), strings.TrimPrefix(path, "~"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return filepath.Clean(abs), nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
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
		return fmt.Errorf("send agent reply note: %w", err)
	}

	result, err := runner.Resume(ctx, session.WorkRoot, run.ProviderSessionID, decision.Message)
	if err != nil {
		return fmt.Errorf("resume codex after support reply: %w", err)
	}

	newRunID := state.RunID(makeID("run"))
	now := time.Now().UTC()
	if err := r.store.SaveRun(state.Run{
		ID:                newRunID,
		SessionID:         session.ID,
		Provider:          session.Provider,
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

func eventProviderForSession(provider string) events.Provider {
	switch provider {
	case "codex", "opencode":
		return events.ProviderCodex
	default:
		return events.ProviderCodex
	}
}
