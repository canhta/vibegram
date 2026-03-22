package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/vibegram/internal/config"
)

func TestLoadFromEnvFileReadsExplicitPath(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "vibegram.env")
	data := []byte("VIBEGRAM_TELEGRAM_BOT_TOKEN=from-file\nVIBEGRAM_TELEGRAM_FORUM_CHAT_ID=-1001234567890\n")
	if err := os.WriteFile(envPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", envPath, err)
	}

	cfg, err := config.LoadFromEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadFromEnvFile() error = %v", err)
	}

	if cfg.Telegram.BotToken != "from-file" {
		t.Fatalf("BotToken = %q, want %q", cfg.Telegram.BotToken, "from-file")
	}
	if cfg.Telegram.ForumChatID != -1001234567890 {
		t.Fatalf("ForumChatID = %d, want %d", cfg.Telegram.ForumChatID, int64(-1001234567890))
	}
}
