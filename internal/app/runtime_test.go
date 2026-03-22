package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type fakeBotClient struct {
	createdTopicNames []string
	sent              []sentMessage
	edited            []editedMessage
	answeredCallbacks []answeredCallback
	nextThreadID      int
	nextMessageID     int
	updates           []telegram.Update
	createTopicErr    error
	deletedTopics     []int
	deleteTopicErrors map[int]error
}

type sentMessage struct {
	messageID int
	chatID    int64
	threadID  *int
	text      string
	markup    *telegram.InlineKeyboardMarkup
}

type editedMessage struct {
	chatID    int64
	messageID int
	text      string
	markup    *telegram.InlineKeyboardMarkup
}

type answeredCallback struct {
	id   string
	text string
}

func (f *fakeBotClient) CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error) {
	if f.createTopicErr != nil {
		return 0, f.createTopicErr
	}
	f.createdTopicNames = append(f.createdTopicNames, name)
	if f.nextThreadID == 0 {
		f.nextThreadID = 42
	}
	return f.nextThreadID, nil
}

func (f *fakeBotClient) SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error {
	f.nextMessageID++
	f.sent = append(f.sent, sentMessage{messageID: f.nextMessageID, chatID: chatID, threadID: threadID, text: text})
	return nil
}

func (f *fakeBotClient) SendMessageCard(ctx context.Context, chatID int64, threadID *int, text string, markup telegram.InlineKeyboardMarkup) (int, error) {
	f.nextMessageID++
	card := markup
	f.sent = append(f.sent, sentMessage{
		messageID: f.nextMessageID,
		chatID:    chatID,
		threadID:  threadID,
		text:      text,
		markup:    &card,
	})
	return f.nextMessageID, nil
}

func (f *fakeBotClient) EditMessageCard(ctx context.Context, chatID int64, messageID int, text string, markup telegram.InlineKeyboardMarkup) error {
	card := markup
	f.edited = append(f.edited, editedMessage{
		chatID:    chatID,
		messageID: messageID,
		text:      text,
		markup:    &card,
	})
	return nil
}

func (f *fakeBotClient) AnswerCallback(ctx context.Context, callbackID, text string) error {
	f.answeredCallbacks = append(f.answeredCallbacks, answeredCallback{id: callbackID, text: text})
	return nil
}

func (f *fakeBotClient) DeleteForumTopic(ctx context.Context, chatID int64, threadID int) error {
	if f.deleteTopicErrors != nil {
		if err, ok := f.deleteTopicErrors[threadID]; ok {
			return err
		}
	}
	f.deletedTopics = append(f.deletedTopics, threadID)
	return nil
}

func (f *fakeBotClient) GetUpdates(ctx context.Context, offset int64) ([]telegram.Update, error) {
	var updates []telegram.Update
	for _, update := range f.updates {
		if update.UpdateID >= offset {
			updates = append(updates, update)
		}
	}
	return updates, nil
}

type fakeCodexSessionRunner struct {
	startPrompt     string
	startWorkDir    string
	resumeSessionID string
	resumePrompt    string
	resumeWorkDir   string
	startResult     codexprovider.SessionResult
	resumeResult    codexprovider.SessionResult
	startRelease    chan struct{}
	startCalled     chan struct{}
}

type fakePolicyEngine struct {
	lastEvent events.NormalizedEvent
	lastSnap  state.Snapshot
	decision  policy.PolicyDecision
	called    bool
}

type fakeSupportResponder struct {
	lastText       string
	reply          string
	validatePrompt string
	validateReply  string
}

func (f *fakeCodexSessionRunner) Start(ctx context.Context, workDir, prompt string) (codexprovider.SessionResult, error) {
	f.startWorkDir = workDir
	f.startPrompt = prompt
	if f.startCalled != nil {
		select {
		case f.startCalled <- struct{}{}:
		default:
		}
	}
	if f.startRelease != nil {
		<-f.startRelease
	}
	return f.startResult, nil
}

func (f *fakeCodexSessionRunner) Resume(ctx context.Context, workDir, providerSessionID, prompt string) (codexprovider.SessionResult, error) {
	f.resumeWorkDir = workDir
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

func (f *fakeSupportResponder) Reply(ctx context.Context, text string) (string, error) {
	f.lastText = text
	return f.reply, nil
}

func (f *fakeSupportResponder) Validate(ctx context.Context, prompt string) (string, error) {
	f.validatePrompt = prompt
	return f.validateReply, nil
}

func TestRuntimeHandleGeneralPlainTextRoutesToSupportAgent(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{}
	codex := &fakeCodexSessionRunner{}
	support := &fakeSupportResponder{reply: "Use /start build health check to create a new session."}

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
		nil,
		support,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "how do i run a new session?",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if support.lastText != "how do i run a new session?" {
		t.Fatalf("support.lastText = %q", support.lastText)
	}
	if len(bot.sent) != 1 || bot.sent[0].text != support.reply {
		t.Fatalf("bot.sent = %+v, want support reply", bot.sent)
	}
	if len(bot.createdTopicNames) != 0 {
		t.Fatalf("createdTopicNames = %v, want none for plain text", bot.createdTopicNames)
	}
	if codex.startPrompt != "" {
		t.Fatalf("startPrompt = %q, want empty for plain text", codex.startPrompt)
	}
}

func TestRuntimeHandleGeneralPlainTextStartDoesNotLaunchSession(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{}
	codex := &fakeCodexSessionRunner{}
	support := &fakeSupportResponder{reply: "Use /start build health check if you want me to create a session topic."}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Runtime: config.RuntimeConfig{
				WorkRoot: root,
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		support,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 2,
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

	if len(bot.createdTopicNames) != 0 {
		t.Fatalf("createdTopicNames = %v, want none for non-slash start", bot.createdTopicNames)
	}
	if codex.startPrompt != "" {
		t.Fatalf("startPrompt = %q, want empty for non-slash start", codex.startPrompt)
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatalf("ReadDir(sessions) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("session file count = %d, want 0 for non-slash start", len(entries))
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
		WorkRoot:       "/tmp/project",
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
		nil,
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
	if codex.resumeWorkDir != "/tmp/project" {
		t.Fatalf("resumeWorkDir = %q, want %q", codex.resumeWorkDir, "/tmp/project")
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

func TestRuntimeHandleGeneralCleanupDeletesRequestedTopics(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{}
	codex := &fakeCodexSessionRunner{}

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
		nil,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 30,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup 12,14",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.deletedTopics) != 2 || bot.deletedTopics[0] != 12 || bot.deletedTopics[1] != 14 {
		t.Fatalf("deletedTopics = %v, want [12 14]", bot.deletedTopics)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0].text, "cleanup: deleted 2 topic") {
		t.Fatalf("cleanup reply = %+v", bot.sent)
	}
}

func TestRuntimeHandleGeneralCleanupAllDeletesPersistedSessionTopics(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sessionA := state.Session{
		ID:             "ses_12",
		ActiveRunID:    "run_12",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 12,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	sessionB := state.Session{
		ID:             "ses_14",
		ActiveRunID:    "run_14",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 14,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	runA := state.Run{ID: "run_12", SessionID: "ses_12", Provider: "codex", Status: state.RunStatusExited}
	runB := state.Run{ID: "run_14", SessionID: "ses_14", Provider: "codex", Status: state.RunStatusExited}

	for _, session := range []state.Session{sessionA, sessionB} {
		if err := store.SaveSession(session); err != nil {
			t.Fatalf("SaveSession() error = %v", err)
		}
	}
	for _, run := range []state.Run{runA, runB} {
		if err := store.SaveRun(run); err != nil {
			t.Fatalf("SaveRun() error = %v", err)
		}
	}

	bot := &fakeBotClient{}
	codex := &fakeCodexSessionRunner{}
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
		nil,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 31,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup all",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.deletedTopics) != 2 || bot.deletedTopics[0] != 12 || bot.deletedTopics[1] != 14 {
		t.Fatalf("deletedTopics = %v, want [12 14]", bot.deletedTopics)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0].text, "cleanup: deleted 2 topic") {
		t.Fatalf("cleanup reply = %+v", bot.sent)
	}
	if _, err := store.LoadSession("ses_12"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadSession(ses_12) err = %v, want not found after cleanup", err)
	}
	if _, err := store.LoadRun("run_12"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadRun(run_12) err = %v, want not found after cleanup", err)
	}
}

func TestRuntimeHandleGeneralCleanupAllIgnoresInvalidTopicIDsAndRemovesState(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sessionA := state.Session{
		ID:             "ses_12",
		ActiveRunID:    "run_12",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 12,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	sessionB := state.Session{
		ID:             "ses_14",
		ActiveRunID:    "run_14",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 14,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	runA := state.Run{ID: "run_12", SessionID: "ses_12", Provider: "codex", Status: state.RunStatusExited}
	runB := state.Run{ID: "run_14", SessionID: "ses_14", Provider: "codex", Status: state.RunStatusExited}

	for _, session := range []state.Session{sessionA, sessionB} {
		if err := store.SaveSession(session); err != nil {
			t.Fatalf("SaveSession() error = %v", err)
		}
	}
	for _, run := range []state.Run{runA, runB} {
		if err := store.SaveRun(run); err != nil {
			t.Fatalf("SaveRun() error = %v", err)
		}
	}

	bot := &fakeBotClient{
		deleteTopicErrors: map[int]error{
			12: fmt.Errorf("telegram api error 400: {\"ok\":false,\"error_code\":400,\"description\":\"Bad Request: TOPIC_ID_INVALID\"}"),
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
		&fakeCodexSessionRunner{},
		nil,
		nil,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 34,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup all",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.deletedTopics) != 1 || bot.deletedTopics[0] != 14 {
		t.Fatalf("deletedTopics = %v, want [14]", bot.deletedTopics)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0].text, "cleanup: deleted 2 topic") {
		t.Fatalf("cleanup reply = %+v", bot.sent)
	}
	if _, err := store.LoadSession("ses_12"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadSession(ses_12) err = %v, want not found after cleanup", err)
	}
	if _, err := store.LoadSession("ses_14"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadSession(ses_14) err = %v, want not found after cleanup", err)
	}
}

func TestRuntimeHandleGeneralCleanupAllDedupesTopicIDs(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	sessionA := state.Session{
		ID:             "ses_a",
		ActiveRunID:    "run_a",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 12,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	sessionB := state.Session{
		ID:             "ses_b",
		ActiveRunID:    "run_b",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 12,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	runA := state.Run{ID: "run_a", SessionID: "ses_a", Provider: "codex", Status: state.RunStatusExited}
	runB := state.Run{ID: "run_b", SessionID: "ses_b", Provider: "codex", Status: state.RunStatusExited}

	for _, session := range []state.Session{sessionA, sessionB} {
		if err := store.SaveSession(session); err != nil {
			t.Fatalf("SaveSession() error = %v", err)
		}
	}
	for _, run := range []state.Run{runA, runB} {
		if err := store.SaveRun(run); err != nil {
			t.Fatalf("SaveRun() error = %v", err)
		}
	}

	bot := &fakeBotClient{}
	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		bot,
		&fakeCodexSessionRunner{},
		nil,
		nil,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 35,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup all",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.deletedTopics) != 1 || bot.deletedTopics[0] != 12 {
		t.Fatalf("deletedTopics = %v, want [12]", bot.deletedTopics)
	}
}

func waitForRuntime(t *testing.T, cond func() bool, label string) {
	t.Helper()

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", label)
}
