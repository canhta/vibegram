package app

import (
	"context"
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
	if !strings.Contains(bot.sent[0].text, "status: ok") {
		t.Fatalf("status reply = %q, want contains %q", bot.sent[0].text, "status: ok")
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
