package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/vibegram/internal/config"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

func TestRuntimeHandleGeneralSlashNewStartsDraftWizardWithoutCreatingTopic(t *testing.T) {
	root := t.TempDir()
	storeDir := t.TempDir()
	store := state.NewStore(storeDir)
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
			Providers: config.ProviderConfig{
				CodexCommand:  "/usr/local/bin/codex",
				ClaudeCommand: "/usr/local/bin/claude",
			},
			Runtime: config.RuntimeConfig{
				WorkRoot: root,
			},
		},
		store,
		bot,
		&fakeCodexSessionRunner{},
		&fakeCodexSessionRunner{},
		nil,
		nil,
	)

	err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new",
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(bot.createdTopicNames) != 0 {
		t.Fatalf("createdTopicNames = %v, want none before launch", bot.createdTopicNames)
	}
	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(bot.sent))
	}
	if !strings.Contains(bot.sent[0].text, "Which agent do you want for this session?") {
		t.Fatalf("first prompt = %q", bot.sent[0].text)
	}
	if bot.sent[0].threadID != nil {
		t.Fatalf("threadID = %v, want General topic message", bot.sent[0].threadID)
	}
	if bot.sent[0].markup == nil {
		t.Fatal("markup = nil, want provider buttons")
	}
	labels := inlineButtonLabels(*bot.sent[0].markup)
	if !containsLabel(labels, "Codex") {
		t.Fatalf("provider labels = %v, want Codex", labels)
	}
	if !containsLabel(labels, "Claude") {
		t.Fatalf("provider labels = %v, want Claude", labels)
	}
	if containsLabel(labels, "OpenCode") {
		t.Fatalf("provider labels = %v, do not want OpenCode", labels)
	}

	entries, err := os.ReadDir(filepath.Join(storeDir, "sessions"))
	if err != nil {
		t.Fatalf("ReadDir(sessions) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("session file count = %d, want 0 before launch", len(entries))
	}
}

func TestRuntimeGeneralWizardUsesHistoryFirstStartChoices(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	projectY := filepath.Join(projectRoot, "project-y")
	for _, path := range []string{projectX, projectY} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("Mkdir(%s) error = %v", path, err)
		}
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

	bot := &fakeBotClient{}
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

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 2,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-provider-codex",
			FromUserID: 1001,
			Data:       "wiz:provider:codex",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[0].messageID,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(provider) error = %v", err)
	}

	if len(bot.sent) != 2 {
		t.Fatalf("sent messages = %d, want 2", len(bot.sent))
	}
	if !strings.Contains(bot.sent[1].text, "Where should we start looking?") {
		t.Fatalf("second prompt = %q", bot.sent[1].text)
	}
	if bot.sent[1].markup == nil {
		t.Fatal("second prompt markup = nil, want location buttons")
	}

	labels := inlineButtonLabels(*bot.sent[1].markup)
	if !containsLabel(labels, "project-x") {
		t.Fatalf("location labels = %v, want project-x", labels)
	}
	if !containsLabel(labels, "project-y") {
		t.Fatalf("location labels = %v, want project-y", labels)
	}
	if !containsLabel(labels, "More Places") {
		t.Fatalf("location labels = %v, want More Places", labels)
	}
}

func TestRuntimeGeneralWizardFallsBackToHomeWhenRecentHistoryIsMissing(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(%s) error = %v", projectX, err)
	}

	missingRecent := filepath.Join(projectRoot, "missing-project")

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_older",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 41,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession(older) error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_recent_missing",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       missingRecent,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession(recent) error = %v", err)
	}

	bot := &fakeBotClient{}
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

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 20,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 21,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-provider-codex",
			FromUserID: 1001,
			Data:       "wiz:provider:codex",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[0].messageID,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(provider) error = %v", err)
	}

	if len(bot.sent) != 2 {
		t.Fatalf("sent messages = %d, want 2", len(bot.sent))
	}
	if !strings.Contains(bot.sent[1].text, "Where should we start looking?") {
		t.Fatalf("second prompt = %q", bot.sent[1].text)
	}
	if bot.sent[1].markup == nil {
		t.Fatal("second prompt markup = nil, want location buttons")
	}

	labels := inlineButtonLabels(*bot.sent[1].markup)
	if !containsLabel(labels, "Home") {
		t.Fatalf("location labels = %v, want Home fallback when recent history is missing", labels)
	}
	if containsLabel(labels, "project-x") {
		t.Fatalf("location labels = %v, do not want stale history suggestions after missing recent history", labels)
	}
}

func TestRuntimeGeneralWizardTaskLaunchesDirectlyWithoutValidation(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectX, "go.mod"), []byte("module example.com/project-x\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectX, "README.md"), []byte("# Project X\n\nExisting repo context.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
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

	bot := &fakeBotClient{nextThreadID: 77}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-direct",
			Message:           "direct launch complete",
		},
	}
	support := &fakeSupportResponder{
		validateReply: "Build a small SEO-focused HTML page in the selected project. Keep it simple, reuse existing repo structure, and explain where the page lives.",
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
		support,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)

	if len(bot.createdTopicNames) != 0 {
		t.Fatalf("createdTopicNames = %v, want none before task send", bot.createdTopicNames)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 10,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "build a small seo page",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(task) error = %v", err)
	}
	if support.validatePrompt != "" {
		t.Fatalf("validatePrompt = %q, want no validation call", support.validatePrompt)
	}
	waitForRuntime(t, func() bool { return codex.startPrompt != "" }, "direct launch")
	if codex.startPrompt != "build a small seo page" {
		t.Fatalf("startPrompt = %q, want raw task text", codex.startPrompt)
	}
	if len(bot.createdTopicNames) != 1 {
		t.Fatalf("createdTopicNames = %v, want one topic after task send", bot.createdTopicNames)
	}
	if !strings.Contains(bot.createdTopicNames[0], "project-x") || !strings.Contains(bot.createdTopicNames[0], "codex") {
		t.Fatalf("topic name = %q, want folder and provider", bot.createdTopicNames[0])
	}
}

func TestRuntimeGeneralWizardCancelDoesNotCreateTopic(t *testing.T) {
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

	bot := &fakeBotClient{}
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

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 12,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-cancel",
			FromUserID: 1001,
			Data:       "wiz:cancel",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    1001,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(cancel) error = %v", err)
	}

	if len(bot.createdTopicNames) != 0 {
		t.Fatalf("createdTopicNames = %v, want none after cancel", bot.createdTopicNames)
	}
	if !strings.Contains(bot.sent[len(bot.sent)-1].text, "Cancelled") {
		t.Fatalf("cancel message = %q", bot.sent[len(bot.sent)-1].text)
	}

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count = %d, want only prior history session", len(sessions))
	}
}

func runGeneralWizardToTaskEntryPrompt(t *testing.T, rt *Runtime, bot *fakeBotClient, userID int64) {
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
			Data:       "wiz:provider:codex",
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

	last := bot.sent[len(bot.sent)-1]
	if strings.TrimSpace(last.text) == "" || !strings.Contains(last.text, "Send the task now to create the topic and launch the session.") {
		t.Fatalf("task prompt = %q, want task entry prompt", last.text)
	}
}

func inlineButtonLabels(markup telegram.InlineKeyboardMarkup) []string {
	labels := make([]string, 0, len(markup.InlineKeyboard)*2)
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			labels = append(labels, button.Text)
		}
	}
	return labels
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
