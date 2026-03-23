package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type fakeBotClient struct {
	mu                sync.RWMutex
	createdTopicNames []string
	sent              []sentMessage
	edited            []editedMessage
	answeredCallbacks []answeredCallback
	setCommandsCalls  []setCommandsCall
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

type setCommandsCall struct {
	chatID   int64
	commands []telegram.BotCommand
}

func (f *fakeBotClient) CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error) {
	if f.createTopicErr != nil {
		return 0, f.createTopicErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createdTopicNames = append(f.createdTopicNames, name)
	if f.nextThreadID == 0 {
		f.nextThreadID = 42
	}
	return f.nextThreadID, nil
}

func (f *fakeBotClient) SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextMessageID++
	f.sent = append(f.sent, sentMessage{messageID: f.nextMessageID, chatID: chatID, threadID: threadID, text: text})
	return nil
}

func (f *fakeBotClient) SendMessageCard(ctx context.Context, chatID int64, threadID *int, text string, markup telegram.InlineKeyboardMarkup) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answeredCallbacks = append(f.answeredCallbacks, answeredCallback{id: callbackID, text: text})
	return nil
}

func (f *fakeBotClient) DeleteForumTopic(ctx context.Context, chatID int64, threadID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteTopicErrors != nil {
		if err, ok := f.deleteTopicErrors[threadID]; ok {
			return err
		}
	}
	f.deletedTopics = append(f.deletedTopics, threadID)
	return nil
}

func (f *fakeBotClient) SetCommands(ctx context.Context, chatID int64, commands []telegram.BotCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	dup := append([]telegram.BotCommand(nil), commands...)
	f.setCommandsCalls = append(f.setCommandsCalls, setCommandsCall{
		chatID:   chatID,
		commands: dup,
	})
	return nil
}

func (f *fakeBotClient) GetUpdates(ctx context.Context, offset int64) ([]telegram.Update, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var updates []telegram.Update
	for _, update := range f.updates {
		if update.UpdateID >= offset {
			updates = append(updates, update)
		}
	}
	return updates, nil
}

func (f *fakeBotClient) sentSnapshot() []sentMessage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]sentMessage(nil), f.sent...)
}

func (f *fakeBotClient) createdTopicNamesSnapshot() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]string(nil), f.createdTopicNames...)
}

func (f *fakeBotClient) editedSnapshot() []editedMessage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]editedMessage(nil), f.edited...)
}

func (f *fakeBotClient) answeredCallbacksSnapshot() []answeredCallback {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]answeredCallback(nil), f.answeredCallbacks...)
}

func (f *fakeBotClient) setCommandsSnapshot() []setCommandsCall {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]setCommandsCall(nil), f.setCommandsCalls...)
}

func (f *fakeBotClient) deletedTopicsSnapshot() []int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]int(nil), f.deletedTopics...)
}

func inlineButtonData(markup telegram.InlineKeyboardMarkup) []string {
	data := make([]string, 0, len(markup.InlineKeyboard)*2)
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			data = append(data, button.CallbackData)
		}
	}
	return data
}

type fakeCodexSessionRunner struct {
	startPrompt       string
	startWorkDir      string
	resumeSessionID   string
	resumePrompt      string
	resumePrompts     []string
	resumeWorkDir     string
	startStreamLines  []string
	resumeStreamLines []string
	startResult       codexprovider.SessionResult
	resumeResult      codexprovider.SessionResult
	resumeResults     []codexprovider.SessionResult
	startRelease      chan struct{}
	startCalled       chan struct{}
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

func (f *fakeCodexSessionRunner) StartStream(ctx context.Context, workDir, prompt string, onLine func(string)) (codexprovider.SessionResult, error) {
	f.startWorkDir = workDir
	f.startPrompt = prompt
	if f.startCalled != nil {
		select {
		case f.startCalled <- struct{}{}:
		default:
		}
	}
	for _, line := range f.startStreamLines {
		onLine(line)
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
	f.resumePrompts = append(f.resumePrompts, prompt)
	if len(f.resumeResults) > 0 {
		result := f.resumeResults[0]
		f.resumeResults = f.resumeResults[1:]
		return result, nil
	}
	return f.resumeResult, nil
}

func (f *fakeCodexSessionRunner) ResumeStream(ctx context.Context, workDir, providerSessionID, prompt string, onLine func(string)) (codexprovider.SessionResult, error) {
	f.resumeWorkDir = workDir
	f.resumeSessionID = providerSessionID
	f.resumePrompt = prompt
	f.resumePrompts = append(f.resumePrompts, prompt)
	for _, line := range f.resumeStreamLines {
		onLine(line)
	}
	if len(f.resumeResults) > 0 {
		result := f.resumeResults[0]
		f.resumeResults = f.resumeResults[1:]
		return result, nil
	}
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

func TestRuntimeHandleSessionTopicMessageAcknowledgesEmptyProviderReply(t *testing.T) {
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
			Message:           "",
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

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 3,
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

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(bot.sent))
	}
	if bot.sent[0].text != "Sent to codex. No visible reply yet." {
		t.Fatalf("sent text = %q, want empty-reply acknowledgement", bot.sent[0].text)
	}
}

func TestRuntimeHandleSessionTopicMessageRendersFilteredCodexEvents(t *testing.T) {
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
			Message:           "Which test framework should I use?",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Which test framework should I use?"}}`,
			}, "\n"),
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

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 4,
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

	if len(bot.sent) != 2 {
		t.Fatalf("sent messages = %d, want 2 filtered events", len(bot.sent))
	}
	if bot.sent[0].text != "Tool: shell — go test ./..." {
		t.Fatalf("sent text[0] = %q", bot.sent[0].text)
	}
	if bot.sent[0].threadID == nil || *bot.sent[0].threadID != 42 {
		t.Fatalf("sent thread[0] = %v, want session topic 42", bot.sent[0].threadID)
	}
	if bot.sent[1].text != "Question: Which test framework should I use?" {
		t.Fatalf("sent text[1] = %q", bot.sent[1].text)
	}
	if bot.sent[1].threadID == nil || *bot.sent[1].threadID != 42 {
		t.Fatalf("sent thread[1] = %v, want session topic 42", bot.sent[1].threadID)
	}
}

func TestRuntimeHandleSessionTopicMessageAllowsAutoReplyOnResumedCodexStop(t *testing.T) {
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
		resumeResults: []codexprovider.SessionResult{
			{
				ProviderSessionID: "thread-123",
				Message:           "Which test framework should I use?",
				RawOutput: strings.Join([]string{
					`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Which test framework should I use?"}}`,
				}, "\n"),
			},
			{
				ProviderSessionID: "thread-123",
				Message:           "I'll use the Go standard library testing package.",
				RawOutput: strings.Join([]string{
					`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"I'll use the Go standard library testing package."}}`,
				}, "\n"),
			},
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
		nil,
		engine,
		nil,
	)
	rt.sessionsByThread[42] = "ses_1"

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 5,
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

	if !engine.called {
		t.Fatal("policy engine was not called on resumed question")
	}
	if len(codex.resumePrompts) != 2 {
		t.Fatalf("resumePrompts = %v, want original prompt and CEO reply", codex.resumePrompts)
	}
	if codex.resumePrompts[0] != "continue" {
		t.Fatalf("first resume prompt = %q, want %q", codex.resumePrompts[0], "continue")
	}
	if codex.resumePrompts[1] != "Use Go's standard library testing package." {
		t.Fatalf("second resume prompt = %q, want CEO reply", codex.resumePrompts[1])
	}
	if len(bot.sent) != 3 {
		t.Fatalf("sent messages = %d, want 3", len(bot.sent))
	}
	if bot.sent[0].text != "Question: Which test framework should I use?" {
		t.Fatalf("sent text[0] = %q", bot.sent[0].text)
	}
	if bot.sent[1].text != "Agent reply: Use Go's standard library testing package." {
		t.Fatalf("sent text[1] = %q", bot.sent[1].text)
	}
	if bot.sent[2].text != "I'll use the Go standard library testing package." {
		t.Fatalf("sent text[2] = %q", bot.sent[2].text)
	}
}

func TestNewRuntimeRestoresSessionTopicMappingsFromStore(t *testing.T) {
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

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 2,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 42,
			Text:     "continue after restart",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if codex.resumeSessionID != "thread-123" {
		t.Fatalf("resumeSessionID = %q, want %q", codex.resumeSessionID, "thread-123")
	}
	if codex.resumePrompt != "continue after restart" {
		t.Fatalf("resumePrompt = %q, want %q", codex.resumePrompt, "continue after restart")
	}
}

func TestRuntimeHandleGeneralCleanupShowsPicker(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, session := range []state.Session{
		{
			ID:             "ses_12",
			ActiveRunID:    "run_12",
			Provider:       "codex",
			GeneralTopicID: 1,
			SessionTopicID: 12,
			WorkRoot:       "/tmp/project-a",
			Status:         state.SessionStatusRunning,
			Phase:          state.SessionPhasePlanning,
		},
		{
			ID:             "ses_14",
			ActiveRunID:    "run_14",
			Provider:       "claude",
			GeneralTopicID: 1,
			SessionTopicID: 14,
			WorkRoot:       "/tmp/project-b",
			Status:         state.SessionStatusRunning,
			Phase:          state.SessionPhasePlanning,
		},
	} {
		if err := store.SaveSession(session); err != nil {
			t.Fatalf("SaveSession() error = %v", err)
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
		UpdateID: 30,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent = %d, want 1 cleanup picker", len(bot.sent))
	}
	if bot.sent[0].markup == nil {
		t.Fatal("cleanup picker markup = nil")
	}
	if !strings.Contains(bot.sent[0].text, "Select session topics to delete") {
		t.Fatalf("cleanup picker text = %q", bot.sent[0].text)
	}
	labels := inlineButtonLabels(*bot.sent[0].markup)
	if !containsLabel(labels, "project-a") {
		t.Fatalf("cleanup labels = %v, want project-a", labels)
	}
	if !containsLabel(labels, "project-b") {
		t.Fatalf("cleanup labels = %v, want project-b", labels)
	}
	if !containsLabel(labels, "All") {
		t.Fatalf("cleanup labels = %v, want All", labels)
	}
	data := inlineButtonData(*bot.sent[0].markup)
	if !containsLabel(data, "cleanup:topic:12") {
		t.Fatalf("cleanup callback data = %v, want cleanup:topic:12", data)
	}
	if !containsLabel(data, "cleanup:all") {
		t.Fatalf("cleanup callback data = %v, want cleanup:all", data)
	}
}

func TestRuntimeHandleGeneralCleanupRepliesWhenNoTopicsExist(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
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
		UpdateID: 31,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/cleanup",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.sent) != 1 || bot.sent[0].markup != nil || !strings.Contains(bot.sent[0].text, "cleanup: no session topics") {
		t.Fatalf("cleanup empty reply = %+v", bot.sent)
	}
}

func TestRuntimeHandleCleanupCallbackDeletesSelectedTopic(t *testing.T) {
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
		UpdateID: 32,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-cleanup-one",
			FromUserID: 1001,
			Data:       "cleanup:topic:12",
			Message: telegram.UpdateMessage{
				MessageID: 9,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if got := bot.answeredCallbacksSnapshot(); len(got) != 1 || got[0].id != "cb-cleanup-one" {
		t.Fatalf("answered callbacks = %+v, want cleanup callback ack", got)
	}
	if len(bot.deletedTopics) != 1 || bot.deletedTopics[0] != 12 {
		t.Fatalf("deletedTopics = %v, want [12]", bot.deletedTopics)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0].text, "cleanup: deleted 1 topic") {
		t.Fatalf("cleanup reply = %+v", bot.sent)
	}
	if _, err := store.LoadSession("ses_12"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadSession(ses_12) err = %v, want not found after cleanup", err)
	}
	if _, err := store.LoadRun("run_12"); !strings.Contains(fmt.Sprint(err), "state record not found") {
		t.Fatalf("LoadRun(run_12) err = %v, want not found after cleanup", err)
	}
	if _, err := store.LoadSession("ses_14"); err != nil {
		t.Fatalf("LoadSession(ses_14) err = %v, want remaining session", err)
	}
}

func TestRuntimeHandleCleanupCallbackAllDeletesPersistedSessionTopics(t *testing.T) {
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
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-cleanup-all",
			FromUserID: 1001,
			Data:       "cleanup:all",
			Message: telegram.UpdateMessage{
				MessageID: 9,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if got := bot.answeredCallbacksSnapshot(); len(got) != 1 || got[0].id != "cb-cleanup-all" {
		t.Fatalf("answered callbacks = %+v, want cleanup callback ack", got)
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

func TestRuntimeHandleCleanupCallbackAllDedupesTopicIDs(t *testing.T) {
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
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-cleanup-all",
			FromUserID: 1001,
			Data:       "cleanup:all",
			Message: telegram.UpdateMessage{
				MessageID: 9,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
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
