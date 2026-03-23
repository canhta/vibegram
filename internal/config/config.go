package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Telegram  TelegramConfig
	OpenAI    OpenAIConfig
	Providers ProviderConfig
	Runtime   RuntimeConfig
}

type TelegramConfig struct {
	BotToken    string
	ForumChatID int64
	AdminIDs    []int64
	OperatorIDs []int64
}

type OpenAIConfig struct {
	APIKey      string
	Model       string
	StrongModel string
	BaseURL     string
}

type ProviderConfig struct {
	ClaudeCommand string
	CodexCommand  string
}

type RuntimeConfig struct {
	WorkRoot string
	StateDir string
	LogLevel string
}

func LoadFromEnv() (Config, error) {
	return loadConfig([]string{".env"}, false)
}

func LoadFromEnvFile(path string) (Config, error) {
	return loadConfig([]string{path}, true)
}

func loadConfig(envFiles []string, override bool) (Config, error) {
	for _, path := range envFiles {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := loadDotEnv(path, override); err != nil {
			return Config{}, err
		}
	}

	botToken, err := requiredStringEnv("VIBEGRAM_TELEGRAM_BOT_TOKEN")
	if err != nil {
		return Config{}, err
	}

	workRoot, err := runtimeWorkRoot()
	if err != nil {
		return Config{}, err
	}

	stateDir, err := runtimeStateDir(workRoot)
	if err != nil {
		return Config{}, err
	}

	forumChatID, err := requiredInt64Env("VIBEGRAM_TELEGRAM_FORUM_CHAT_ID")
	if err != nil {
		return Config{}, err
	}

	adminIDs, err := optionalInt64ListEnv("VIBEGRAM_TELEGRAM_ADMIN_IDS")
	if err != nil {
		return Config{}, err
	}

	operatorIDs, err := optionalInt64ListEnv("VIBEGRAM_TELEGRAM_OPERATOR_IDS")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Telegram: TelegramConfig{
			BotToken:    botToken,
			ForumChatID: forumChatID,
			AdminIDs:    adminIDs,
			OperatorIDs: operatorIDs,
		},
		OpenAI: OpenAIConfig{
			APIKey:      os.Getenv("OPENAI_API_KEY"),
			Model:       envOrDefault("VIBEGRAM_OPENAI_MODEL", "gpt-5-mini"),
			StrongModel: envOrDefault("VIBEGRAM_OPENAI_STRONG_MODEL", "gpt-5"),
			BaseURL:     envOrDefault("VIBEGRAM_OPENAI_BASE_URL", "https://api.openai.com/v1"),
		},
		Providers: ProviderConfig{
			ClaudeCommand: os.Getenv("VIBEGRAM_PROVIDER_CLAUDE_CMD"),
			CodexCommand:  os.Getenv("VIBEGRAM_PROVIDER_CODEX_CMD"),
		},
		Runtime: RuntimeConfig{
			WorkRoot: workRoot,
			StateDir: stateDir,
			LogLevel: envOrDefault("VIBEGRAM_LOG_LEVEL", "info"),
		},
	}, nil
}

func requiredStringEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}

	return value, nil
}

func requiredInt64Env(key string) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, fmt.Errorf("%s is required", key)
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid int64: %w", key, err)
	}

	return parsed, nil
}

func optionalInt64ListEnv(key string) ([]int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil, nil
	}

	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		parsed, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a comma-separated list of int64 values: %w", key, err)
		}
		ids = append(ids, parsed)
	}

	return ids, nil
}

func runtimeWorkRoot() (string, error) {
	workRoot := envOrDefault("VIBEGRAM_WORK_ROOT", "")
	if workRoot == "" {
		var err error
		workRoot, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve work root: %w", err)
		}
	}

	abs, err := filepath.Abs(workRoot)
	if err != nil {
		return "", fmt.Errorf("resolve work root: %w", err)
	}

	return filepath.Clean(abs), nil
}

func runtimeStateDir(workRoot string) (string, error) {
	stateDir := envOrDefault("VIBEGRAM_STATE_DIR", filepath.Join(workRoot, "state"))
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return "", fmt.Errorf("resolve state dir: %w", err)
	}

	return filepath.Clean(abs), nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func loadDotEnv(path string, override bool) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !override && os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from %s: %w", key, path, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	return nil
}
