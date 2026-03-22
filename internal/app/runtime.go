package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
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

type streamingSessionRunner interface {
	StartStream(ctx context.Context, workDir, prompt string, onLine func(string)) (codexprovider.SessionResult, error)
	ResumeStream(ctx context.Context, workDir, providerSessionID, prompt string, onLine func(string)) (codexprovider.SessionResult, error)
}

type policyEngine interface {
	Evaluate(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (policy.PolicyDecision, error)
}

type supportResponder interface {
	Reply(ctx context.Context, text string) (string, error)
	Validate(ctx context.Context, prompt string) (string, error)
}

type Runtime struct {
	cfg              config.Config
	store            *state.Store
	bot              botClient
	codex            sessionRunner
	claude           sessionRunner
	policy           policyEngine
	support          supportResponder
	auth             *telegram.Authorizer
	sessionsByThread map[int]state.SessionID
	draftsByUser     map[int64]generalDraft
	mu               sync.RWMutex
}

func NewRuntime(cfg config.Config, store *state.Store, bot botClient, codex sessionRunner, claude sessionRunner, policy policyEngine, support supportResponder) *Runtime {
	rt := &Runtime{
		cfg:              cfg,
		store:            store,
		bot:              bot,
		codex:            codex,
		claude:           claude,
		policy:           policy,
		support:          support,
		auth:             telegram.NewAuthorizer(cfg.Telegram.AdminIDs, cfg.Telegram.OperatorIDs),
		sessionsByThread: make(map[int]state.SessionID),
		draftsByUser:     make(map[int64]generalDraft),
	}
	if store != nil {
		sessions, err := store.ListSessions()
		if err == nil {
			for _, session := range sessions {
				if session.SessionTopicID <= 1 {
					continue
				}
				rt.sessionsByThread[int(session.SessionTopicID)] = session.ID
			}
		}
	}
	return rt
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
		if draft, ok := r.draftForUser(userID); ok {
			return r.handleGeneralDraftText(ctx, chatID, draft, text)
		}
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

	case text == "new":
		return r.startGeneralDraft(ctx, chatID, userID)

	case text == "cleanup":
		return r.bot.SendMessage(ctx, chatID, nil, "Usage: /cleanup <topic_id[,topic_id]> or /cleanup all")

	case strings.HasPrefix(text, "cleanup "):
		return r.cleanupTopics(ctx, chatID, strings.TrimSpace(strings.TrimPrefix(text, "cleanup ")))

	case text == "start":
		return r.bot.SendMessage(ctx, chatID, nil, "Use /new to start a new session.")

	default:
		return nil
	}
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

func (r *Runtime) draftForUser(userID int64) (generalDraft, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	draft, ok := r.draftsByUser[userID]
	return draft, ok
}

func (r *Runtime) saveDraft(draft generalDraft) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.draftsByUser[draft.UserID] = draft
}

func (r *Runtime) clearDraft(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.draftsByUser, userID)
}
