package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type fakeBotClient struct {
	createdTopicNames []string
	sent              []sentMessage
	nextThreadID      int
	updates           []telegram.Update
}

type sentMessage struct {
	chatID   int64
	threadID *int
	text     string
}

func (f *fakeBotClient) CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error) {
	f.createdTopicNames = append(f.createdTopicNames, name)
	if f.nextThreadID == 0 {
		f.nextThreadID = 42
	}
	return f.nextThreadID, nil
}

func (f *fakeBotClient) SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error {
	f.sent = append(f.sent, sentMessage{chatID: chatID, threadID: threadID, text: text})
	return nil
}

func (f *fakeBotClient) GetUpdates(ctx context.Context, offset int64) ([]telegram.Update, error) {
	updates := f.updates
	f.updates = nil
	return updates, nil
}

type fakeCodexSessionRunner struct {
	startPrompt       string
	resumeSessionID   string
	resumePrompt      string
	startResult       codexprovider.SessionResult
	resumeResult      codexprovider.SessionResult
}

type fakePolicyEngine struct {
	lastEvent   events.NormalizedEvent
	lastSnap    state.Snapshot
	decision    policy.PolicyDecision
	called      bool
}

func (f *fakeCodexSessionRunner) Start(ctx context.Context, prompt string) (codexprovider.SessionResult, error) {
	f.startPrompt = prompt
	return f.startResult, nil
}

func (f *fakeCodexSessionRunner) Resume(ctx context.Context, providerSessionID, prompt string) (codexprovider.SessionResult, error) {
	f.resumeSessionID = providerSessionID
	f.resumePrompt = prompt
	return f.resumeResult, nil
}

func (f *fakePolicyEngine) Evaluate(ctx context.Context, snap state.Snapshot, event events.NormalizedEvent) (policy.PolicyDecision, error) {
	f.called = true
	f.lastSnap = snap
	f.lastEvent = event
	return f.decision, nil
}

func TestRuntimeHandleGeneralStartCreatesSessionAndSendsMessages(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "assistant final message",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		bot,
		codex,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "start build health check",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if codex.startPrompt != "build health check" {
		t.Fatalf("startPrompt = %q, want %q", codex.startPrompt, "build health check")
	}
	if len(bot.createdTopicNames) != 1 || bot.createdTopicNames[0] != "build health check" {
		t.Fatalf("createdTopicNames = %v, want [build health check]", bot.createdTopicNames)
	}
	if len(bot.sent) < 2 {
		t.Fatalf("sent messages = %d, want at least 2", len(bot.sent))
	}
	if !strings.Contains(bot.sent[0].text, "Session started") {
		t.Fatalf("first message = %q, want session-start notice", bot.sent[0].text)
	}
	if bot.sent[len(bot.sent)-1].text != "assistant final message" {
		t.Fatalf("last message = %q, want %q", bot.sent[len(bot.sent)-1].text, "assistant final message")
	}

	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatalf("ReadDir(sessions) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("session file count = %d, want 1", len(entries))
	}
}

func TestRuntimeHandleSessionTopicMessageResumesProviderSession(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	session := state.Session{
		ID:             "ses_1",
		ActiveRunID:    "run_1",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 42,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	run := state.Run{
		ID:                "run_1",
		SessionID:         "ses_1",
		Provider:          "codex",
		ProviderSessionID: "thread-123",
		Status:            state.RunStatusExited,
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	bot := &fakeBotClient{}
	codex := &fakeCodexSessionRunner{
		resumeResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "resumed reply",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		bot,
		codex,
		nil,
	)
	rt.sessionsByThread[42] = "ses_1"

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 2,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 42,
			Text:     "continue",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if codex.resumeSessionID != "thread-123" {
		t.Fatalf("resumeSessionID = %q, want %q", codex.resumeSessionID, "thread-123")
	}
	if codex.resumePrompt != "continue" {
		t.Fatalf("resumePrompt = %q, want %q", codex.resumePrompt, "continue")
	}
	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(bot.sent))
	}
	if bot.sent[0].text != "resumed reply" {
		t.Fatalf("sent text = %q, want %q", bot.sent[0].text, "resumed reply")
	}
	if bot.sent[0].threadID == nil || *bot.sent[0].threadID != 42 {
		t.Fatalf("threadID = %v, want 42", bot.sent[0].threadID)
	}
}

func TestRuntimeStartSessionAutoRepliesToSafeQuestion(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "Which test framework should I use?",
		},
		resumeResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "I'll use the Go standard library testing package.",
		},
	}
	engine := &fakePolicyEngine{
		decision: policy.PolicyDecision{
			Action:  roles.ActionReply,
			Message: "Use Go's standard library testing package.",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		bot,
		codex,
		engine,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "start choose test framework",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if !engine.called {
		t.Fatal("expected support-role policy to be called")
	}
	if engine.lastEvent.EventType != events.EventTypeQuestion {
		t.Fatalf("EventType = %q, want %q", engine.lastEvent.EventType, events.EventTypeQuestion)
	}
	if codex.resumePrompt != "Use Go's standard library testing package." {
		t.Fatalf("resumePrompt = %q, want %q", codex.resumePrompt, "Use Go's standard library testing package.")
	}
	if len(bot.sent) < 4 {
		t.Fatalf("sent messages = %d, want at least 4", len(bot.sent))
	}
	if bot.sent[1].text != "Which test framework should I use?" {
		t.Fatalf("question message = %q", bot.sent[1].text)
	}
	if bot.sent[2].text != "Agent reply: Use Go's standard library testing package." {
		t.Fatalf("agent reply note = %q", bot.sent[2].text)
	}
	if bot.sent[3].text != "I'll use the Go standard library testing package." {
		t.Fatalf("follow-up message = %q", bot.sent[3].text)
	}
}
