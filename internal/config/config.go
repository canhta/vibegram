package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type SandboxProfile string

const (
	SandboxProfileWorkspaceWrite                   SandboxProfile = "workspace_write"
	SandboxProfileWorkspaceWriteNetworkOff         SandboxProfile = "workspace_write_network_off"
	SandboxProfileWorkspaceWriteAllowlistedNetwork SandboxProfile = "workspace_write_allowlisted_network"
	SandboxProfileFullAccess                       SandboxProfile = "full_access"
)

type Config struct {
	Telegram  TelegramConfig
	OpenAI    OpenAIConfig
	Providers ProviderConfig
	Runtime   RuntimeConfig
	Sandbox   SandboxConfig
}

type TelegramConfig struct {
	BotToken    string
	ForumChatID int64
}

type OpenAIConfig struct {
	APIKey string
	Model  string
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

type SandboxConfig struct {
	DefaultProfile                 SandboxProfile
	AllowlistedNetworkDestinations []string
}

func LoadFromEnv() (Config, error) {
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

	profile, err := sandboxProfileFromEnv()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Telegram: TelegramConfig{
			BotToken:    botToken,
			ForumChatID: forumChatID,
		},
		OpenAI: OpenAIConfig{
			APIKey: os.Getenv("OPENAI_API_KEY"),
			Model:  envOrDefault("VIBEGRAM_OPENAI_MODEL", "gpt-5"),
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
		Sandbox: SandboxConfig{
			DefaultProfile:                 profile,
			AllowlistedNetworkDestinations: parseCSVEnv("VIBEGRAM_SANDBOX_ALLOWLISTED_NETWORK_DESTINATIONS"),
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

func sandboxProfileFromEnv() (SandboxProfile, error) {
	profile := SandboxProfile(envOrDefault("VIBEGRAM_SANDBOX_DEFAULT_PROFILE", string(SandboxProfileWorkspaceWriteNetworkOff)))
	switch profile {
	case SandboxProfileWorkspaceWrite, SandboxProfileWorkspaceWriteNetworkOff, SandboxProfileWorkspaceWriteAllowlistedNetwork, SandboxProfileFullAccess:
		return profile, nil
	default:
		return "", fmt.Errorf("VIBEGRAM_SANDBOX_DEFAULT_PROFILE must be one of %q, %q, %q, or %q",
			SandboxProfileWorkspaceWrite,
			SandboxProfileWorkspaceWriteNetworkOff,
			SandboxProfileWorkspaceWriteAllowlistedNetwork,
			SandboxProfileFullAccess,
		)
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	parsed := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		parsed = append(parsed, value)
	}

	return parsed
}
