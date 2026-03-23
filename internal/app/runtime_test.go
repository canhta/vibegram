package app

import (
	"context"
	"errors"
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
	sendErrors        map[int]error
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
	threadKey := 0
	if threadID != nil {
		threadKey = *threadID
	}
	if f.sendErrors != nil {
		if err, ok := f.sendErrors[threadKey]; ok {
			return err
		}
	}
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

func sentMessagesContain(messages []sentMessage, want string) bool {
	for _, message := range messages {
		if message.text == want {
			return true
		}
	}
	return false
}

func sentMessagesContainSubstring(messages []sentMessage, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.text, want) {
			return true
		}
	}
	return false
}

func findSentMessage(messages []sentMessage, want string) (sentMessage, bool) {
	for _, message := range messages {
		if message.text == want {
			return message, true
		}
	}
	return sentMessage{}, false
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
	startErr          error
	resumeErr         error
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
	if f.startErr != nil {
		return codexprovider.SessionResult{}, f.startErr
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
	if f.startErr != nil {
		return codexprovider.SessionResult{}, f.startErr
	}
	return f.startResult, nil
}

func (f *fakeCodexSessionRunner) Resume(ctx context.Context, workDir, providerSessionID, prompt string) (codexprovider.SessionResult, error) {
	f.resumeWorkDir = workDir
	f.resumeSessionID = providerSessionID
	f.resumePrompt = prompt
	f.resumePrompts = append(f.resumePrompts, prompt)
	if f.resumeErr != nil {
		return codexprovider.SessionResult{}, f.resumeErr
	}
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
	if f.resumeErr != nil {
		return codexprovider.SessionResult{}, f.resumeErr
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

func TestRuntimeHandleSessionTopicResumeFailureMarksSessionFailedWithoutReturningError(t *testing.T) {
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
		resumeErr: errors.New("codex exit 1: unexpected status 413 Payload Too Large"),
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
		t.Fatalf("HandleUpdate() error = %v, want nil", err)
	}

	if !sentMessagesContainSubstring(bot.sent, "resume failed: codex exit 1: unexpected status 413 Payload Too Large") {
		t.Fatalf("sent messages = %+v, want resume failure notice", bot.sent)
	}
	resumeFailure, found := findSentMessage(bot.sent, "resume failed: codex exit 1: unexpected status 413 Payload Too Large")
	if !found {
		t.Fatalf("sent messages = %+v, want exact resume failure message", bot.sent)
	}
	if resumeFailure.threadID == nil || *resumeFailure.threadID != 42 {
		t.Fatalf("threadID = %v, want 42", resumeFailure.threadID)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support escalated in project codex ") {
		t.Fatalf("sent messages = %+v, want General escalation awareness", bot.sent)
	}

	updatedSession, err := store.LoadSession("ses_1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if updatedSession.Status != state.SessionStatusFailed {
		t.Fatalf("session status = %q, want %q", updatedSession.Status, state.SessionStatusFailed)
	}
	if updatedSession.ActiveRunID == "run_1" {
		t.Fatalf("ActiveRunID = %q, want a new failed run", updatedSession.ActiveRunID)
	}

	failedRun, err := store.LoadRun(updatedSession.ActiveRunID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if failedRun.Status != state.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", failedRun.Status, state.RunStatusFailed)
	}
	if failedRun.ProviderSessionID != "thread-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", failedRun.ProviderSessionID, "thread-123")
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

	if len(bot.sent) < 3 {
		t.Fatalf("sent messages = %d, want header card plus filtered events", len(bot.sent))
	}
	toolMessage, found := findSentMessage(bot.sent, "Tool: shell — go test ./...")
	if !found {
		t.Fatalf("sent messages = %+v, want tool event", bot.sent)
	}
	if toolMessage.threadID == nil || *toolMessage.threadID != 42 {
		t.Fatalf("sent thread(tool) = %v, want session topic 42", toolMessage.threadID)
	}
	questionMessage, found := findSentMessage(bot.sent, "Question: Which test framework should I use?")
	if !found {
		t.Fatalf("sent messages = %+v, want question event", bot.sent)
	}
	if questionMessage.threadID == nil || *questionMessage.threadID != 42 {
		t.Fatalf("sent thread(question) = %v, want session topic 42", questionMessage.threadID)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support: idle") {
		t.Fatalf("sent messages = %+v, want session header card for existing session", bot.sent)
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
	if len(bot.sent) != 5 {
		t.Fatalf("sent messages = %d, want 5", len(bot.sent))
	}
	if !sentMessagesContain(bot.sent, "Question: Which test framework should I use?") {
		t.Fatalf("sent messages = %+v, want question message", bot.sent)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support replied in project codex ") {
		t.Fatalf("sent messages = %+v, want General support awareness", bot.sent)
	}
	if !sentMessagesContain(bot.sent, "Agent reply: Use Go's standard library testing package.") {
		t.Fatalf("sent messages = %+v, want agent reply note", bot.sent)
	}
	if !sentMessagesContain(bot.sent, "I'll use the Go standard library testing package.") {
		t.Fatalf("sent messages = %+v, want follow-up message", bot.sent)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support: idle") {
		t.Fatalf("sent messages = %+v, want session header card for existing session", bot.sent)
	}
}

func TestRuntimeMaybeAutoReplyResumeFailureMarksSessionFailedWithoutReturningError(t *testing.T) {
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
		resumeErr: errors.New("codex exit 1: unexpected status 413 Payload Too Large"),
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

	err := rt.maybeAutoReplyForEvent(context.Background(), -1001234567890, 42, &session, run, events.NormalizedEvent{
		EventType: events.EventTypeQuestion,
		Summary:   "Which test framework should I use?",
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("maybeAutoReplyForEvent() error = %v, want nil", err)
	}

	if !sentMessagesContain(bot.sent, "Agent reply: Use Go's standard library testing package.") {
		t.Fatalf("sent messages = %+v, want agent reply note", bot.sent)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support replied in project codex ") || !sentMessagesContainSubstring(bot.sent, "Use Go's standard library testing package.") {
		t.Fatalf("sent messages = %+v, want reply awareness", bot.sent)
	}
	if !sentMessagesContainSubstring(bot.sent, "Support escalated in project codex ") || !sentMessagesContainSubstring(bot.sent, "unexpected status 413 Payload Too Large") {
		t.Fatalf("sent messages = %+v, want escalation awareness", bot.sent)
	}
	if !sentMessagesContainSubstring(bot.sent, "resume failed: codex exit 1: unexpected status 413 Payload Too Large") {
		t.Fatalf("sent messages = %+v, want resume failure", bot.sent)
	}

	updatedSession, err := store.LoadSession("ses_1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if updatedSession.Status != state.SessionStatusFailed {
		t.Fatalf("session status = %q, want %q", updatedSession.Status, state.SessionStatusFailed)
	}
	if updatedSession.LastRoleUsed != "support" {
		t.Fatalf("LastRoleUsed = %q, want %q", updatedSession.LastRoleUsed, "support")
	}
	if updatedSession.ReplyAttemptCount != 1 {
		t.Fatalf("ReplyAttemptCount = %d, want 1", updatedSession.ReplyAttemptCount)
	}

	failedRun, err := store.LoadRun(updatedSession.ActiveRunID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if failedRun.Status != state.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", failedRun.Status, state.RunStatusFailed)
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
			ID:                "ses_12",
			ActiveRunID:       "run_12",
			Provider:          "codex",
			GeneralTopicID:    1,
			SessionTopicID:    12,
			SessionTopicTitle: "Actual Topic A",
			WorkRoot:          "/tmp/project-a",
			Status:            state.SessionStatusRunning,
			Phase:             state.SessionPhasePlanning,
		},
		{
			ID:                "ses_14",
			ActiveRunID:       "run_14",
			Provider:          "claude",
			GeneralTopicID:    1,
			SessionTopicID:    14,
			SessionTopicTitle: "Actual Topic B",
			WorkRoot:          "/tmp/project-b",
			Status:            state.SessionStatusRunning,
			Phase:             state.SessionPhasePlanning,
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
	if !containsLabel(labels, "Actual Topic A") {
		t.Fatalf("cleanup labels = %v, want Actual Topic A", labels)
	}
	if !containsLabel(labels, "Actual Topic B") {
		t.Fatalf("cleanup labels = %v, want Actual Topic B", labels)
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

func TestCleanupLabelFallsBackToDerivedTopicTitle(t *testing.T) {
	label := cleanupLabel(42, []state.Session{{
		ID:             "ses_1774230019339001903",
		Provider:       "codex",
		WorkRoot:       "/home/ubuntu/projects/vibegram",
		SessionTopicID: 42,
	}})

	if label != "vibegram codex 1903" {
		t.Fatalf("cleanupLabel() = %q, want %q", label, "vibegram codex 1903")
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

func TestRuntimeHandleGeneralStatusCreatesAndRefreshesPersistentControlCard(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	session := state.Session{
		ID:                "ses_1",
		ActiveRunID:       "run_1",
		Provider:          "codex",
		GeneralTopicID:    1,
		SessionTopicID:    42,
		SessionTopicTitle: "Topic Alpha",
		Status:            state.SessionStatusRunning,
		Phase:             state.SessionPhaseEditing,
		LastGoal:          "tighten launch flow",
		LastBlocker:       "stale blocker",
		LastQuestion:      "old question",
		WorkRoot:          "/tmp/project",
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveSnapshot("ses_1", state.Snapshot{
		SessionID:              "ses_1",
		Phase:                  string(state.SessionPhaseEditing),
		Status:                 string(state.SessionStatusRunning),
		LastBlocker:            "waiting on provider output",
		LastQuestion:           "which branch should we ship?",
		RecentFilesSummary:     "internal/app/runtime.go",
		RecentTestsSummary:     "go test ./... green",
		ReplyAttemptCount:      1,
		EscalationState:        string(state.EscalationStateNone),
		SupportState:           string(state.SupportStateAskHuman),
		SupportDecisionSummary: "choose the release branch before shipping",
		HumanActionNeeded:      true,
		UpdatedAt:              time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
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

	first := telegram.Update{
		UpdateID: 40,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/status",
		},
	}
	if err := rt.HandleUpdate(context.Background(), first); err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1 control card", len(bot.sent))
	}
	if bot.sent[0].markup == nil {
		t.Fatal("status control card markup = nil")
	}
	if !strings.Contains(bot.sent[0].text, "General control room") {
		t.Fatalf("control card text = %q, want title", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Needs you now: 1") {
		t.Fatalf("control card text = %q, want attention count", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "waiting on provider output") {
		t.Fatalf("control card text = %q, want snapshot blocker", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "go test ./... green") {
		t.Fatalf("control card text = %q, want snapshot tests", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Support: ask human") {
		t.Fatalf("control card text = %q, want support state", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Decision: choose the release branch before shipping") {
		t.Fatalf("control card text = %q, want support decision summary", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Needs you now: yes") {
		t.Fatalf("control card text = %q, want per-session human action marker", bot.sent[0].text)
	}
	if strings.Contains(bot.sent[0].text, "stale blocker") {
		t.Fatalf("control card text = %q, want snapshot values to win over stale session state", bot.sent[0].text)
	}

	cursor, err := store.LoadCursor("general_control_card_message_id")
	if err != nil {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if cursor != int64(bot.sent[0].messageID) {
		t.Fatalf("cursor = %d, want %d", cursor, bot.sent[0].messageID)
	}

	restarted := NewRuntime(
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
	if err := restarted.HandleUpdate(context.Background(), first); err != nil {
		t.Fatalf("HandleUpdate() after restart error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages after restart = %d, want 1 create and 1 refresh", len(bot.sent))
	}
	if len(bot.edited) != 1 {
		t.Fatalf("edited messages after restart = %d, want 1 refresh", len(bot.edited))
	}
	if bot.edited[0].messageID != bot.sent[0].messageID {
		t.Fatalf("edited messageID = %d, want %d", bot.edited[0].messageID, bot.sent[0].messageID)
	}
	if !strings.Contains(bot.edited[0].text, "General control room") {
		t.Fatalf("edited control card text = %q, want title", bot.edited[0].text)
	}
}

func TestRuntimeHandleGeneralStatusShowsNoActiveSessions(t *testing.T) {
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
		UpdateID: 41,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/status",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1 no-active-sessions control card", len(bot.sent))
	}
	if bot.sent[0].markup == nil {
		t.Fatal("status control card markup = nil")
	}
	if !strings.Contains(bot.sent[0].text, "No active sessions.") {
		t.Fatalf("control card text = %q, want no-active-sessions state", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Use /new to start one.") {
		t.Fatalf("control card text = %q, want next-step guidance", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Needs you now: 0") {
		t.Fatalf("control card text = %q, want zero-attention count", bot.sent[0].text)
	}
}

func TestRuntimeHandleGeneralStatusFallsBackToSessionSupportStateWithoutSnapshot(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := store.SaveSession(state.Session{
		ID:                     "ses_2",
		ActiveRunID:            "run_2",
		Provider:               "codex",
		GeneralTopicID:         1,
		SessionTopicID:         44,
		SessionTopicTitle:      "Topic Beta",
		Status:                 state.SessionStatusBlocked,
		Phase:                  state.SessionPhaseWaiting,
		LastGoal:               "close the launch checklist",
		LastQuestion:           "should we cut the patch now?",
		SupportState:           state.SupportStateEscalated,
		SupportDecisionSummary: "waiting for human approval to release",
		HumanActionNeeded:      true,
		WorkRoot:               "/tmp/project-beta",
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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
		UpdateID: 42,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/status",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1 control card", len(bot.sent))
	}
	if !strings.Contains(bot.sent[0].text, "Topic Beta | codex | blocked") {
		t.Fatalf("control card text = %q, want session summary", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Support: escalated") {
		t.Fatalf("control card text = %q, want fallback support state", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Decision: waiting for human approval to release") {
		t.Fatalf("control card text = %q, want fallback support decision", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "Needs you now: yes") {
		t.Fatalf("control card text = %q, want fallback human action marker", bot.sent[0].text)
	}
}

func TestRuntimeHandleGeneralStatusIncludesFailedEscalatedSessionNeedingHumanAction(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := store.SaveSession(state.Session{
		ID:                     "ses_failed",
		ActiveRunID:            "run_failed",
		Provider:               "codex",
		GeneralTopicID:         1,
		SessionTopicID:         45,
		Status:                 state.SessionStatusFailed,
		Phase:                  state.SessionPhasePlanning,
		WorkRoot:               "/tmp/project-failed",
		EscalationState:        state.EscalationStateNeeded,
		SupportState:           state.SupportStateEscalated,
		SupportDecisionSummary: "provider offline",
		HumanActionNeeded:      true,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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
		UpdateID: 43,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/status",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	text := bot.sent[0].text
	if strings.Contains(text, "No active sessions.") {
		t.Fatalf("control card text = %q, want failed session to stay visible", text)
	}
	if !strings.Contains(text, "Needs you now: 1") {
		t.Fatalf("control card text = %q, want attention count", text)
	}
	if !strings.Contains(text, "project-failed codex") {
		t.Fatalf("control card text = %q, want derived topic title", text)
	}
	if !strings.Contains(text, "| failed") {
		t.Fatalf("control card text = %q, want failed status", text)
	}
}

func TestGeneralControlCardTextTruncatesLongSessionDetails(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	longGoal := strings.Repeat("ship the release safely ", 220)
	longBlocker := strings.Repeat("telegram says message is too long ", 180)
	longQuestion := strings.Repeat("should we cut the release now ", 180)
	longDecision := strings.Repeat("support says keep the control room readable ", 180)
	longFiles := strings.Repeat("internal/app/runtime_control_card.go ", 120)
	longTests := strings.Repeat("go test ./... ", 220)

	session := state.Session{
		ID:                     "ses_long",
		ActiveRunID:            "run_long",
		Provider:               "codex",
		GeneralTopicID:         1,
		SessionTopicID:         99,
		SessionTopicTitle:      "Project control room",
		Status:                 state.SessionStatusBlocked,
		Phase:                  state.SessionPhaseRunningTests,
		LastGoal:               longGoal,
		LastBlocker:            longBlocker,
		LastQuestion:           longQuestion,
		RecentFilesSummary:     longFiles,
		RecentTestsSummary:     longTests,
		SupportState:           state.SupportStateAskHuman,
		SupportDecisionSummary: longDecision,
		HumanActionNeeded:      true,
		EscalationState:        state.EscalationStateNeeded,
		WorkRoot:               "/tmp/project-control-room",
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		&fakeBotClient{},
		&fakeCodexSessionRunner{},
		nil,
		nil,
		nil,
	)

	text, _, err := rt.generalControlCardText()
	if err != nil {
		t.Fatalf("generalControlCardText() error = %v", err)
	}

	if len(text) > 4096 {
		t.Fatalf("len(control card text) = %d, want <= 4096", len(text))
	}
	if !strings.Contains(text, "General control room") {
		t.Fatalf("control card text = %q, want title", text)
	}
	if !strings.Contains(text, "Needs you now: 1") {
		t.Fatalf("control card text = %q, want attention summary", text)
	}
	if !strings.Contains(text, "Project control room | codex | blocked") {
		t.Fatalf("control card text = %q, want session summary", text)
	}
	if !strings.Contains(text, "Support: ask human") {
		t.Fatalf("control card text = %q, want support state", text)
	}
	if !strings.Contains(text, "Decision: ") {
		t.Fatalf("control card text = %q, want support decision line", text)
	}
	if !strings.Contains(text, "...") {
		t.Fatalf("control card text = %q, want visible truncation marker", text)
	}
}

func TestRenderSessionHeaderCardTruncatesLongFields(t *testing.T) {
	text := renderSessionHeaderCard(state.Session{
		Provider:               "codex",
		WorkRoot:               "/tmp/" + strings.Repeat("deep-folder-", 60),
		LastGoal:               strings.Repeat("finish the calm delegation polish ", 180),
		Status:                 state.SessionStatusBlocked,
		Phase:                  state.SessionPhaseRunningTests,
		SupportState:           state.SupportStateEscalated,
		SupportDecisionSummary: strings.Repeat("ask the human to confirm the reinstall path ", 180),
		HumanActionNeeded:      true,
	})

	if len(text) > 4096 {
		t.Fatalf("len(session header text) = %d, want <= 4096", len(text))
	}
	if !strings.Contains(text, "Agent: codex") {
		t.Fatalf("session header text = %q, want agent line", text)
	}
	if !strings.Contains(text, "State: blocked / running_tests") {
		t.Fatalf("session header text = %q, want state line", text)
	}
	if !strings.Contains(text, "Support: escalated") {
		t.Fatalf("session header text = %q, want support state line", text)
	}
	if !strings.Contains(text, "Human action: yes") {
		t.Fatalf("session header text = %q, want human action line", text)
	}
	if !strings.Contains(text, "...") {
		t.Fatalf("session header text = %q, want visible truncation marker", text)
	}
}

func TestRuntimeApplySupportDecisionCreatesHeaderRefreshesGeneralCardAndPostsAwareness(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	session := state.Session{
		ID:                "ses_3",
		ActiveRunID:       "run_3",
		Provider:          "codex",
		GeneralTopicID:    1,
		SessionTopicID:    46,
		SessionTopicTitle: "Topic Gamma",
		Status:            state.SessionStatusRunning,
		Phase:             state.SessionPhaseEditing,
		LastGoal:          "finish the control room",
		WorkRoot:          "/tmp/project-gamma",
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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

	if err := rt.refreshGeneralControlCard(context.Background(), -1001234567890); err != nil {
		t.Fatalf("refreshGeneralControlCard() error = %v", err)
	}

	snap, err := rt.loadSessionSnapshot(session)
	if err != nil {
		t.Fatalf("loadSessionSnapshot() error = %v", err)
	}
	if err := rt.applySupportDecision(context.Background(), -1001234567890, 46, &session, snap, state.SupportStateReplied, "Use Go's testing package.", false); err != nil {
		t.Fatalf("applySupportDecision() error = %v", err)
	}

	updated, err := store.LoadSession("ses_3")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if updated.SessionHeaderMessageID == 0 {
		t.Fatal("SessionHeaderMessageID = 0, want header to be created for existing session")
	}

	if !sentMessagesContain(bot.sentSnapshot(), "Support replied in Topic Gamma: Use Go's testing package.") {
		t.Fatalf("sent messages = %+v, want General awareness", bot.sentSnapshot())
	}

	edits := bot.editedSnapshot()
	if len(edits) == 0 {
		t.Fatal("editedSnapshot() = 0, want General control card refresh")
	}
	if !strings.Contains(edits[len(edits)-1].text, "Support: replied") {
		t.Fatalf("control card edit = %q, want support state", edits[len(edits)-1].text)
	}
	if !strings.Contains(edits[len(edits)-1].text, "Decision: Use Go's testing package.") {
		t.Fatalf("control card edit = %q, want support decision", edits[len(edits)-1].text)
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
