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
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type fakeBotClient struct {
	createdTopicNames []string
	sent              []sentMessage
	nextThreadID      int
	updates           []telegram.Update
	createTopicErr    error
	deletedTopics     []int
	deleteTopicErrors map[int]error
}

type sentMessage struct {
	chatID   int64
	threadID *int
	text     string
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
	f.sent = append(f.sent, sentMessage{chatID: chatID, threadID: threadID, text: text})
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
	resumeSessionID string
	resumePrompt    string
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

func (f *fakeCodexSessionRunner) Start(ctx context.Context, prompt string) (codexprovider.SessionResult, error) {
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

	waitForRuntime(t, func() bool { return codex.startPrompt == "build health check" }, "codex start prompt")
	if len(bot.createdTopicNames) != 1 || bot.createdTopicNames[0] != "build health check" {
		t.Fatalf("createdTopicNames = %v, want [build health check]", bot.createdTopicNames)
	}
	waitForRuntime(t, func() bool { return len(bot.sent) >= 3 }, "session start messages")
	if !strings.Contains(bot.sent[0].text, "Session started") {
		t.Fatalf("first message = %q, want session-start notice", bot.sent[0].text)
	}
	if bot.sent[1].text != "Session starting: build health check" {
		t.Fatalf("second message = %q, want %q", bot.sent[1].text, "Session starting: build health check")
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

func TestRuntimeHandleGeneralStartPersistsStateBeforeCodexFinishes(t *testing.T) {
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
		startRelease: make(chan struct{}),
		startCalled:  make(chan struct{}, 1),
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

	done := make(chan error, 1)
	go func() {
		done <- rt.HandleUpdate(context.Background(), telegram.Update{
			UpdateID: 20,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 1,
				Text:     "start build health check",
			},
		})
	}()

	select {
	case <-codex.startCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("start runner was not called")
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		entries, err := os.ReadDir(filepath.Join(root, "sessions"))
		if err != nil {
			t.Fatalf("ReadDir(sessions) error = %v", err)
		}
		if len(entries) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session state was not persisted before codex finished")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(bot.sent) < 2 {
		t.Fatalf("sent messages = %d, want at least 2 before codex finishes", len(bot.sent))
	}
	if bot.sent[1].threadID == nil || *bot.sent[1].threadID != 42 {
		t.Fatalf("threadID = %v, want 42", bot.sent[1].threadID)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleUpdate() error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("HandleUpdate did not return while codex was still starting")
	}

	close(codex.startRelease)
	time.Sleep(50 * time.Millisecond)
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

	waitForRuntime(t, func() bool { return engine.called }, "support-role policy call")
	if engine.lastEvent.EventType != events.EventTypeQuestion {
		t.Fatalf("EventType = %q, want %q", engine.lastEvent.EventType, events.EventTypeQuestion)
	}
	waitForRuntime(t, func() bool { return codex.resumePrompt == "Use Go's standard library testing package." }, "support-role follow-up resume")
	waitForRuntime(t, func() bool { return len(bot.sent) >= 5 }, "support-role message fanout")
	if bot.sent[2].text != "Which test framework should I use?" {
		t.Fatalf("question message = %q", bot.sent[2].text)
	}
	if bot.sent[3].text != "Agent reply: Use Go's standard library testing package." {
		t.Fatalf("agent reply note = %q", bot.sent[3].text)
	}
	if bot.sent[4].text != "I'll use the Go standard library testing package." {
		t.Fatalf("follow-up message = %q", bot.sent[4].text)
	}
}

func TestRuntimeHandleGeneralStartReportsCreateTopicFailureWithoutCrashing(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	bot := &fakeBotClient{createTopicErr: fmt.Errorf("telegram api error 400: Bad Request: not enough rights to create a topic")}
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
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 9,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "start build health check",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v, want nil", err)
	}
	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(bot.sent))
	}
	if !strings.Contains(strings.ToLower(bot.sent[0].text), "start failed") {
		t.Fatalf("failure message = %q, want start failed notice", bot.sent[0].text)
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
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 30,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "cleanup 12,14",
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

func TestRuntimeHandleGeneralSlashStartSupportsBotMention(t *testing.T) {
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
		UpdateID: 32,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/start@vibeloop_bot build health check",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	waitForRuntime(t, func() bool { return codex.startPrompt == "build health check" }, "slash start prompt")
}

func TestRuntimeHandleGeneralCleanUpAllAliasDeletesPersistedSessionTopics(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	session := state.Session{
		ID:             "ses_12",
		ActiveRunID:    "run_12",
		Provider:       "codex",
		GeneralTopicID: 1,
		SessionTopicID: 12,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}
	run := state.Run{ID: "run_12", SessionID: "ses_12", Provider: "codex", Status: state.RunStatusExited}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
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
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 33,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "clean up all",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if len(bot.deletedTopics) != 1 || bot.deletedTopics[0] != 12 {
		t.Fatalf("deletedTopics = %v, want [12]", bot.deletedTopics)
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
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 34,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "cleanup all",
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
