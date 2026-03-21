package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/app"
	"github.com/canhta/vibegram/internal/config"
)

func TestBootstrapFromEnvBuildsApp(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	workRoot := t.TempDir()

	t.Setenv("VIBEGRAM_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "-1001234567890")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("VIBEGRAM_OPENAI_MODEL", "gpt-5")
	t.Setenv("VIBEGRAM_PROVIDER_CLAUDE_CMD", "/usr/local/bin/claude")
	t.Setenv("VIBEGRAM_PROVIDER_CODEX_CMD", "/usr/local/bin/codex")
	t.Setenv("VIBEGRAM_WORK_ROOT", workRoot)
	t.Setenv("VIBEGRAM_STATE_DIR", stateDir)
	t.Setenv("VIBEGRAM_SANDBOX_ALLOWLISTED_NETWORK_DESTINATIONS", "api.openai.com, registry.npmjs.org")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.Telegram.ForumChatID != -1001234567890 {
		t.Fatalf("ForumChatID = %d, want %d", cfg.Telegram.ForumChatID, int64(-1001234567890))
	}

	if cfg.Sandbox.DefaultProfile != config.SandboxProfileWorkspaceWriteNetworkOff {
		t.Fatalf("DefaultProfile = %q, want %q", cfg.Sandbox.DefaultProfile, config.SandboxProfileWorkspaceWriteNetworkOff)
	}

	wantAllowlist := []string{"api.openai.com", "registry.npmjs.org"}
	if !reflect.DeepEqual(cfg.Sandbox.AllowlistedNetworkDestinations, wantAllowlist) {
		t.Fatalf("AllowlistedNetworkDestinations = %#v, want %#v", cfg.Sandbox.AllowlistedNetworkDestinations, wantAllowlist)
	}

	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if application.Config().Runtime.StateDir != stateDir {
		t.Fatalf("StateDir = %q, want %q", application.Config().Runtime.StateDir, stateDir)
	}
}

func TestLoadFromEnvRejectsInvalidForumChatID(t *testing.T) {
	t.Setenv("VIBEGRAM_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "not-a-chat-id")

	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "VIBEGRAM_TELEGRAM_FORUM_CHAT_ID") {
		t.Fatalf("LoadFromEnv() error = %q, want variable name", err)
	}
}

func TestLoadFromEnvRejectsMissingBotToken(t *testing.T) {
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "-1001234567890")

	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "VIBEGRAM_TELEGRAM_BOT_TOKEN") {
		t.Fatalf("LoadFromEnv() error = %q, want variable name", err)
	}
}

func TestRunCreatesStateDirAndStopsOnContextCancel(t *testing.T) {
	cfg := bootstrapConfig(t)
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- application.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if hasDir(filepath.Join(cfg.Runtime.StateDir, "sessions")) && hasDir(filepath.Join(cfg.Runtime.StateDir, "runs")) {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("state layout under %q was not created before deadline", cfg.Runtime.StateDir)
		}

		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func bootstrapConfig(t *testing.T) config.Config {
	t.Helper()

	stateDir := filepath.Join(t.TempDir(), "state")
	workRoot := t.TempDir()

	t.Setenv("VIBEGRAM_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "-1001234567890")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("VIBEGRAM_PROVIDER_CLAUDE_CMD", "/usr/local/bin/claude")
	t.Setenv("VIBEGRAM_PROVIDER_CODEX_CMD", "/usr/local/bin/codex")
	t.Setenv("VIBEGRAM_WORK_ROOT", workRoot)
	t.Setenv("VIBEGRAM_STATE_DIR", stateDir)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	return cfg
}
