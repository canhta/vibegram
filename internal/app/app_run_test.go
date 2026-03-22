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
					Text:     "status",
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
