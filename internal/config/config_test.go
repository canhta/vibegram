package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/canhta/vibegram/internal/config"
)

func TestLoadFromEnvParsesOpenAIBaseURLAndTelegramRoleIDs(t *testing.T) {
	t.Setenv("VIBEGRAM_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID", "-1001234567890")
	t.Setenv("VIBEGRAM_TELEGRAM_ADMIN_IDS", "1001,1002")
	t.Setenv("VIBEGRAM_TELEGRAM_OPERATOR_IDS", "2001,2002")
	t.Setenv("VIBEGRAM_OPENAI_BASE_URL", "https://example.test/v1")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.OpenAI.BaseURL != "https://example.test/v1" {
		t.Fatalf("BaseURL = %q, want %q", cfg.OpenAI.BaseURL, "https://example.test/v1")
	}

	if !reflect.DeepEqual(cfg.Telegram.AdminIDs, []int64{1001, 1002}) {
		t.Fatalf("AdminIDs = %v, want %v", cfg.Telegram.AdminIDs, []int64{1001, 1002})
	}

	if !reflect.DeepEqual(cfg.Telegram.OperatorIDs, []int64{2001, 2002}) {
		t.Fatalf("OperatorIDs = %v, want %v", cfg.Telegram.OperatorIDs, []int64{2001, 2002})
	}
}

func TestLoadFromEnvLoadsDotEnvFileWhenProcessEnvIsMissing(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	data := []byte("VIBEGRAM_TELEGRAM_BOT_TOKEN=from-dotenv\nVIBEGRAM_TELEGRAM_FORUM_CHAT_ID=-1001234567890\n")
	if err := os.WriteFile(envPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(%q) error = %v", tmp, err)
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.Telegram.BotToken != "from-dotenv" {
		t.Fatalf("BotToken = %q, want %q", cfg.Telegram.BotToken, "from-dotenv")
	}
	if cfg.Telegram.ForumChatID != -1001234567890 {
		t.Fatalf("ForumChatID = %d, want %d", cfg.Telegram.ForumChatID, int64(-1001234567890))
	}
}
