package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

func TestAppRunPollsTelegramAndHandlesStatusCommand(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	bot := &fakeBotClient{
		updates: []telegram.Update{
			{
				UpdateID: 1,
				Message: telegram.UpdateMessage{
					UserID:   1001,
					ChatID:   -1001234567890,
					ThreadID: 1,
					Text:     "/status",
				},
			},
		},
	}
	codex := &fakeCodexSessionRunner{}

	app := &App{
		cfg: config.Config{
			Telegram: config.TelegramConfig{
				BotToken:    "telegram-token",
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Runtime: config.RuntimeConfig{
				WorkRoot: root,
				StateDir: root,
			},
		},
		store: store,
		bot:   bot,
		codex: codex,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(bot.sent) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}

	if len(bot.sent) == 0 {
		t.Fatal("expected a Telegram reply to be sent")
	}
	if bot.sent[0].markup == nil {
		t.Fatal("expected /status to send a control card with markup")
	}
	if !strings.Contains(bot.sent[0].text, "General control room") {
		t.Fatalf("status card = %q, want control-room summary", bot.sent[0].text)
	}
	if !strings.Contains(bot.sent[0].text, "No active sessions.") {
		t.Fatalf("status card = %q, want no-active-sessions state", bot.sent[0].text)
	}
}

func TestAppRunRegistersTelegramCommandsBeforePolling(t *testing.T) {
	root := t.TempDir()
	store := state.NewStore(root)
	bot := &fakeBotClient{
		updates: []telegram.Update{
			{
				UpdateID: 1,
				Message: telegram.UpdateMessage{
					UserID:   1001,
					ChatID:   -1001234567890,
					ThreadID: 1,
					Text:     "/status",
				},
			},
		},
	}

	app := &App{
		cfg: config.Config{
			Telegram: config.TelegramConfig{
				BotToken:    "telegram-token",
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Runtime: config.RuntimeConfig{
				WorkRoot: root,
				StateDir: root,
			},
		},
		store: store,
		bot:   bot,
		codex: &fakeCodexSessionRunner{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(bot.setCommandsCalls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}

	if len(bot.setCommandsCalls) != 1 {
		t.Fatalf("setCommandsCalls = %d, want 1", len(bot.setCommandsCalls))
	}
	got := bot.setCommandsCalls[0]
	if got.chatID != -1001234567890 {
		t.Fatalf("chatID = %d, want -1001234567890", got.chatID)
	}
	if len(got.commands) != 3 {
		t.Fatalf("commands len = %d, want 3", len(got.commands))
	}
	if got.commands[0].Command != "new" || got.commands[1].Command != "status" || got.commands[2].Command != "cleanup" {
		t.Fatalf("commands = %+v, want new/status/cleanup", got.commands)
	}
}

func TestAppRunPersistsTelegramOffsetAcrossRestart(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken:    "telegram-token",
			ForumChatID: -1001234567890,
			AdminIDs:    []int64{1001},
		},
		Runtime: config.RuntimeConfig{
			WorkRoot: root,
			StateDir: root,
		},
	}

	runOnce := func(bot *fakeBotClient) []sentMessage {
		app := &App{
			cfg:   cfg,
			store: state.NewStore(root),
			bot:   bot,
			codex: &fakeCodexSessionRunner{},
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- app.Run(ctx)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for len(bot.sent) == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop after cancellation")
		}

		return bot.sent
	}

	first := runOnce(&fakeBotClient{
		updates: []telegram.Update{{
			UpdateID: 5,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 1,
				Text:     "/status",
			},
		}},
	})
	if len(first) != 1 {
		t.Fatalf("first sent len = %d, want 1", len(first))
	}

	second := runOnce(&fakeBotClient{
		updates: []telegram.Update{{
			UpdateID: 5,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 1,
				Text:     "/status",
			},
		}},
	})
	if len(second) != 0 {
		t.Fatalf("second sent len = %d, want 0 because offset should skip replay", len(second))
	}
}

func TestAppRunPersistsTelegramOffsetWhenSessionResumeFails(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken:    "telegram-token",
			ForumChatID: -1001234567890,
			AdminIDs:    []int64{1001},
		},
		Runtime: config.RuntimeConfig{
			WorkRoot: root,
			StateDir: root,
		},
	}

	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_1",
		ActiveRunID:    "run_1",
		Provider:       "codex",
		WorkRoot:       "/tmp/project",
		GeneralTopicID: 1,
		SessionTopicID: 42,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveRun(state.Run{
		ID:                "run_1",
		SessionID:         "ses_1",
		Provider:          "codex",
		ProviderSessionID: "thread-123",
		Status:            state.RunStatusExited,
	}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	runOnce := func(bot *fakeBotClient) []sentMessage {
		app := &App{
			cfg:   cfg,
			store: state.NewStore(root),
			bot:   bot,
			codex: &fakeCodexSessionRunner{
				resumeErr: errors.New("codex exit 1: unexpected status 413 Payload Too Large"),
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- app.Run(ctx)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for len(bot.sent) == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop after cancellation")
		}

		return bot.sentSnapshot()
	}

	first := runOnce(&fakeBotClient{
		updates: []telegram.Update{{
			UpdateID: 7,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 42,
				Text:     "continue",
			},
		}},
	})
	if len(first) < 3 {
		t.Fatalf("first sent len = %d, want header, awareness, and resume failure", len(first))
	}
	if !hasAppRunSentSubstring(first, "Support escalated in project codex ") {
		t.Fatalf("first sent = %+v, want General escalation awareness", first)
	}
	if !hasAppRunSentSubstring(first, "resume failed:") {
		t.Fatalf("first sent = %+v, want resume failure", first)
	}

	cursor, err := state.NewStore(root).LoadCursor("telegram_updates")
	if err != nil {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if cursor != 8 {
		t.Fatalf("cursor = %d, want 8", cursor)
	}

	second := runOnce(&fakeBotClient{
		updates: []telegram.Update{{
			UpdateID: 7,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 42,
				Text:     "continue",
			},
		}},
	})
	if len(second) != 0 {
		t.Fatalf("second sent len = %d, want 0 because offset should skip replay", len(second))
	}
}

func hasAppRunSentSubstring(messages []sentMessage, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.text, want) {
			return true
		}
	}
	return false
}

func TestAppRunKeepsPollingWhenSessionTopicIsGone(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken:    "telegram-token",
			ForumChatID: -1001234567890,
			AdminIDs:    []int64{1001},
		},
		Runtime: config.RuntimeConfig{
			WorkRoot: root,
			StateDir: root,
		},
	}

	store := state.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_1",
		ActiveRunID:    "run_1",
		Provider:       "codex",
		WorkRoot:       "/tmp/project",
		GeneralTopicID: 1,
		SessionTopicID: 42,
		Status:         state.SessionStatusRunning,
		Phase:          state.SessionPhasePlanning,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveRun(state.Run{
		ID:                "run_1",
		SessionID:         "ses_1",
		Provider:          "codex",
		ProviderSessionID: "thread-123",
		Status:            state.RunStatusExited,
	}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	bot := &fakeBotClient{
		sendErrors: map[int]error{
			42: errors.New("telegram api error 400: {\"ok\":false,\"error_code\":400,\"description\":\"Bad Request: message thread not found\"}"),
		},
		updates: []telegram.Update{
			{
				UpdateID: 10,
				Message: telegram.UpdateMessage{
					UserID:   1001,
					ChatID:   -1001234567890,
					ThreadID: 42,
					Text:     "continue",
				},
			},
			{
				UpdateID: 11,
				Message: telegram.UpdateMessage{
					UserID:   1001,
					ChatID:   -1001234567890,
					ThreadID: 1,
					Text:     "/status",
				},
			},
		},
	}

	app := &App{
		cfg:   cfg,
		store: state.NewStore(root),
		bot:   bot,
		codex: &fakeCodexSessionRunner{
			resumeStreamLines: []string{
				`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(bot.sentSnapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}

	sent := bot.sentSnapshot()
	if len(sent) == 0 {
		t.Fatal("expected General status reply after stale session topic failure")
	}
	if !strings.Contains(sent[len(sent)-1].text, "General control room") {
		t.Fatalf("last sent text = %q, want control room card", sent[len(sent)-1].text)
	}
	if strings.Contains(sent[len(sent)-1].text, "No active sessions.") {
		t.Fatalf("last sent text = %q, want failed session to remain visible for operator attention", sent[len(sent)-1].text)
	}
	if !strings.Contains(sent[len(sent)-1].text, "Needs you now: 1") {
		t.Fatalf("last sent text = %q, want attention count after stale topic cleanup", sent[len(sent)-1].text)
	}
	if !strings.Contains(sent[len(sent)-1].text, "| failed") {
		t.Fatalf("last sent text = %q, want failed session status after stale topic cleanup", sent[len(sent)-1].text)
	}

	cursor, err := state.NewStore(root).LoadCursor("telegram_updates")
	if err != nil {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if cursor != 12 {
		t.Fatalf("cursor = %d, want 12", cursor)
	}

	session, err := state.NewStore(root).LoadSession("ses_1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if session.SessionTopicID != 0 {
		t.Fatalf("session topic id = %d, want 0 after topic is gone", session.SessionTopicID)
	}
	if session.Status != state.SessionStatusFailed {
		t.Fatalf("session status = %q, want %q", session.Status, state.SessionStatusFailed)
	}
}
