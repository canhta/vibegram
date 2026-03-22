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

func TestRuntimeGeneralWizardLaunchPersistsRunBeforeProviderFinishes(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	storeDir := t.TempDir()
	store := state.NewStore(storeDir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskPrompt(t, rt, bot, 1001, "build health check")

	done := make(chan error, 1)
	go func() {
		done <- rt.HandleUpdate(context.Background(), telegram.Update{
			UpdateID: 22,
			CallbackQuery: &telegram.CallbackQuery{
				ID:         "cb-launch",
				FromUserID: 1001,
				Data:       "wiz:launch",
				Message: telegram.UpdateMessage{
					MessageID: bot.sent[len(bot.sent)-1].messageID,
					ChatID:    -1001234567890,
					ThreadID:  1,
				},
			},
		})
	}()

	select {
	case <-codex.startCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("start runner was not called")
	}

	runs, err := os.ReadDir(filepath.Join(storeDir, "runs"))
	if err != nil {
		t.Fatalf("ReadDir(runs) error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run file count = %d, want 1 before provider finishes", len(runs))
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleUpdate(launch) error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("HandleUpdate(launch) did not return while provider was still starting")
	}

	close(codex.startRelease)
	time.Sleep(50 * time.Millisecond)
}

func TestRuntimeGeneralWizardCanLaunchClaude(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{}
	claude := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "claude-session-123",
			Message:           "claude final message",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand:  "/usr/local/bin/codex",
				ClaudeCommand: "/usr/local/bin/claude",
			},
		},
		store,
		bot,
		codex,
		claude,
		nil,
		nil,
	)

	runGeneralWizardToChosenProviderTask(t, rt, bot, 1001, "claude", "ship marketing page")
	launchDraftFromGeneral(t, rt, bot, 1001)

	waitForRuntime(t, func() bool { return claude.startPrompt == "ship marketing page" }, "claude launch")
	if codex.startPrompt != "" {
		t.Fatalf("codex.startPrompt = %q, want empty when claude is selected", codex.startPrompt)
	}
	if claude.startWorkDir != projectX {
		t.Fatalf("claude.startWorkDir = %q, want %q", claude.startWorkDir, projectX)
	}
	waitForRuntime(t, func() bool {
		return len(bot.sent) >= 6 && bot.sent[len(bot.sent)-1].text == "claude final message"
	}, "claude final message")
}

func TestRuntimeGeneralWizardLaunchAutoRepliesToSafeQuestion(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		engine,
		nil,
	)

	runGeneralWizardToTaskPrompt(t, rt, bot, 1001, "choose test framework")
	launchDraftFromGeneral(t, rt, bot, 1001)

	waitForRuntime(t, func() bool { return engine.called }, "support-role policy call")
	if engine.lastEvent.EventType != events.EventTypeQuestion {
		t.Fatalf("EventType = %q, want %q", engine.lastEvent.EventType, events.EventTypeQuestion)
	}
	waitForRuntime(t, func() bool { return codex.resumePrompt == "Use Go's standard library testing package." }, "support-role follow-up resume")
	waitForRuntime(t, func() bool { return len(bot.sent) >= 8 }, "support-role message fanout")
	if codex.resumeWorkDir != projectX {
		t.Fatalf("resumeWorkDir = %q, want %q", codex.resumeWorkDir, projectX)
	}
	if bot.sent[len(bot.sent)-3].text != "Which test framework should I use?" {
		t.Fatalf("question message = %q", bot.sent[len(bot.sent)-3].text)
	}
	if bot.sent[len(bot.sent)-2].text != "Agent reply: Use Go's standard library testing package." {
		t.Fatalf("agent reply note = %q", bot.sent[len(bot.sent)-2].text)
	}
	if bot.sent[len(bot.sent)-1].text != "I'll use the Go standard library testing package." {
		t.Fatalf("follow-up message = %q", bot.sent[len(bot.sent)-1].text)
	}
}

func TestRuntimeGeneralWizardLaunchReportsCreateTopicFailureWithoutCrashing(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{createTopicErr: fmt.Errorf("telegram api error 400: Bad Request: not enough rights to create a topic")}
	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		&fakeCodexSessionRunner{},
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskPrompt(t, rt, bot, 1001, "build health check")
	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-launch",
			FromUserID: 1001,
			Data:       "wiz:launch",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate() error = %v, want nil", err)
	}
	if len(bot.sent) == 0 {
		t.Fatal("sent messages = 0, want launch failed notice")
	}
	if !strings.Contains(strings.ToLower(bot.sent[len(bot.sent)-1].text), "launch failed") {
		t.Fatalf("failure message = %q, want launch failed notice", bot.sent[len(bot.sent)-1].text)
	}
}

func TestRuntimeHandleGeneralSlashNewSupportsBotMention(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
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
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 40,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new@vibeloop_bot",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new@mention) error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want provider prompt", len(bot.sent))
	}
	if !strings.Contains(bot.sent[0].text, "Which agent do you want for this session?") {
		t.Fatalf("provider prompt = %q", bot.sent[0].text)
	}
}

func runGeneralWizardToChosenProviderTask(t *testing.T, rt *Runtime, bot *fakeBotClient, userID int64, provider, task string) {
	t.Helper()

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 4,
		Message: telegram.UpdateMessage{
			UserID:   userID,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-provider",
			FromUserID: userID,
			Data:       "wiz:provider:" + provider,
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(provider) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-start-choice",
			FromUserID: userID,
			Data:       "wiz:start:0",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(start choice) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-choose-here",
			FromUserID: userID,
			Data:       "wiz:browse:choose",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(choose here) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 8,
		Message: telegram.UpdateMessage{
			UserID:   userID,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     task,
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(task) error = %v", err)
	}
}

func launchDraftFromGeneral(t *testing.T, rt *Runtime, bot *fakeBotClient, userID int64) {
	t.Helper()

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-launch",
			FromUserID: userID,
			Data:       "wiz:launch",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(launch) error = %v", err)
	}
}
