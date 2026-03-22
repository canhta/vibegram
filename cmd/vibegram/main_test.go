package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunBootstrapsAndStopsOnCancel(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	workRoot := t.TempDir()

	t.Setenv("VIBEGRAM_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "-1001234567890")
	t.Setenv("VIBEGRAM_PROVIDER_CLAUDE_CMD", "/usr/local/bin/claude")
	t.Setenv("VIBEGRAM_PROVIDER_CODEX_CMD", "/usr/local/bin/codex")
	t.Setenv("VIBEGRAM_WORK_ROOT", workRoot)
	t.Setenv("VIBEGRAM_STATE_DIR", stateDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(stateDir); err == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("state dir %q was not created before deadline", stateDir)
		}

		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not stop after context cancellation")
	}
}

func TestRunReturnsConfigErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := run(ctx)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "VIBEGRAM_TELEGRAM_BOT_TOKEN") {
		t.Fatalf("run() error = %q, want missing token error", err)
	}
}

func TestShouldUseSignalContext(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: nil, want: true},
		{args: []string{}, want: true},
		{args: []string{"daemon"}, want: true},
		{args: []string{"daemon", "--env-file", "/etc/vibegram/env"}, want: true},
		{args: []string{"init"}, want: false},
		{args: []string{"service", "install"}, want: false},
	}

	for _, tt := range tests {
		if got := shouldUseSignalContext(tt.args); got != tt.want {
			t.Fatalf("shouldUseSignalContext(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}
