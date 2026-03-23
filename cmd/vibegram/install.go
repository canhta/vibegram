package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	defaultEnvFile     = "/etc/vibegram/env"
	defaultUnitFile    = "/etc/systemd/system/vibegram.service"
	defaultWorkRoot    = "/var/lib/vibegram"
	defaultLogLevel    = "info"
	defaultOpenAIURL   = "https://api.openai.com/v1"
	defaultOpenAIModel = "gpt-5"
)

func runInstall(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, deps cliDeps) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)

	envFile := fs.String("env-file", defaultEnvFile, "path to vibegram env file")
	unitFile := fs.String("unit-file", defaultUnitFile, "path to systemd unit file")
	serviceUser := fs.String("user", "", "system service account")
	workRoot := fs.String("work-root", defaultWorkRoot, "service work root")
	stateDir := fs.String("state-dir", "", "service state directory")
	botToken := fs.String("bot-token", "", "Telegram bot token")
	chatID := fs.String("chat-id", "", "Telegram forum chat ID")
	adminIDs := fs.String("admin-ids", "", "admin Telegram user IDs")
	operatorIDs := fs.String("operator-ids", "", "operator Telegram user IDs")
	codexCmd := fs.String("codex-cmd", "", "absolute path to codex command")
	claudeCmd := fs.String("claude-cmd", "", "absolute path to claude command")
	openAIAPIKey := fs.String("openai-api-key", "", "OpenAI-compatible API key")
	openAIBaseURL := fs.String("openai-base-url", "", "OpenAI-compatible base URL")
	openAIModel := fs.String("openai-model", "", "OpenAI-compatible model")

	if err := fs.Parse(args); err != nil {
		return err
	}

	chosenUser, err := defaultInstallUser(deps, *serviceUser)
	if err != nil {
		return err
	}

	serviceAccount, err := inspectServiceAccount(chosenUser, *workRoot, deps)
	if err != nil {
		return err
	}

	values := initValues{
		BotToken:      strings.TrimSpace(*botToken),
		ForumChatID:   strings.TrimSpace(*chatID),
		AdminIDs:      strings.TrimSpace(*adminIDs),
		OperatorIDs:   strings.TrimSpace(*operatorIDs),
		CodexCommand:  resolveCommand(*codexCmd, detectCommandForUserHome(serviceAccount.HomeDir, "codex", deps)),
		ClaudeCommand: resolveCommand(*claudeCmd, detectCommandForUserHome(serviceAccount.HomeDir, "claude", deps)),
		WorkRoot:      filepath.Clean(*workRoot),
		StateDir:      strings.TrimSpace(*stateDir),
		LogLevel:      defaultLogLevel,
		OpenAIAPIKey:  fallbackString(*openAIAPIKey, deps.getenv("OPENAI_API_KEY")),
		OpenAIBaseURL: fallbackString(*openAIBaseURL, deps.getenv("VIBEGRAM_OPENAI_BASE_URL")),
		OpenAIModel:   fallbackString(*openAIModel, deps.getenv("VIBEGRAM_OPENAI_MODEL")),
	}

	if values.StateDir == "" {
		values.StateDir = filepath.Join(values.WorkRoot, "state")
	}
	if values.OpenAIBaseURL == "" && values.OpenAIAPIKey != "" {
		values.OpenAIBaseURL = defaultOpenAIURL
	}
	if values.OpenAIModel == "" && values.OpenAIAPIKey != "" {
		values.OpenAIModel = defaultOpenAIModel
	}

	reader := bufio.NewReader(stdin)
	if strings.TrimSpace(values.BotToken) == "" {
		if values.BotToken, err = promptValue(reader, stdout, "Telegram bot token", values.BotToken); err != nil {
			return err
		}
	}
	if strings.TrimSpace(values.ForumChatID) == "" {
		if values.ForumChatID, err = promptValue(reader, stdout, "Telegram forum chat ID", values.ForumChatID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(values.AdminIDs) == "" {
		if values.AdminIDs, err = promptValue(reader, stdout, "Admin Telegram user IDs (comma-separated, optional)", values.AdminIDs); err != nil {
			return err
		}
	}
	if strings.TrimSpace(values.OperatorIDs) == "" {
		if values.OperatorIDs, err = promptValue(reader, stdout, "Operator Telegram user IDs (comma-separated, optional)", values.OperatorIDs); err != nil {
			return err
		}
	}

	if strings.TrimSpace(values.BotToken) == "" {
		return fmt.Errorf("telegram bot token is required")
	}
	if strings.TrimSpace(values.ForumChatID) == "" {
		return fmt.Errorf("telegram forum chat ID is required")
	}

	values.SystemPath = buildServicePath(values.CodexCommand, values.ClaudeCommand)

	if err := writeEnvFile(*envFile, values); err != nil {
		return err
	}

	if err := installService(ctx, stdout, deps, *envFile, *unitFile, values.WorkRoot, chosenUser); err != nil {
		return err
	}
	if err := deps.runCommand(ctx, "systemctl", "enable", "--now", "vibegram"); err != nil {
		return err
	}
	return deps.runCommand(ctx, "systemctl", "status", "vibegram", "--no-pager")
}

func defaultInstallUser(deps cliDeps, requested string) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested), nil
	}

	if deps.getenv != nil {
		if sudoUser := strings.TrimSpace(deps.getenv("SUDO_USER")); sudoUser != "" && sudoUser != "root" {
			return sudoUser, nil
		}
	}

	if deps.currentUser == nil {
		return defaultServiceUser, nil
	}
	current, err := deps.currentUser()
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if strings.TrimSpace(current.Username) == "" {
		return defaultServiceUser, nil
	}
	if strings.TrimSpace(current.Username) == "root" {
		return defaultServiceUser, nil
	}
	return current.Username, nil
}

func inspectServiceAccount(name, workRoot string, deps cliDeps) (serviceAccount, error) {
	if deps.lookupUser == nil {
		return serviceAccount{Name: name, HomeDir: workRoot, GroupRef: name}, nil
	}

	account, err := lookupServiceAccount(name, deps)
	if err == nil {
		if strings.TrimSpace(account.HomeDir) == "" {
			account.HomeDir = workRoot
		}
		return account, nil
	}

	var unknown user.UnknownUserError
	if !errors.As(err, &unknown) {
		return serviceAccount{}, fmt.Errorf("lookup user %s: %w", name, err)
	}

	return serviceAccount{Name: name, HomeDir: workRoot, GroupRef: name}, nil
}

func detectCommandForUserHome(homeDir, name string, deps cliDeps) string {
	if deps.lookPath != nil {
		if path, err := deps.lookPath(name); err == nil {
			return path
		}
	}

	candidates := []string{
		filepath.Join(homeDir, ".local", "bin", name),
		filepath.Join(homeDir, "bin", name),
	}
	patterns := []string{
		filepath.Join(homeDir, ".nvm", "versions", "node", "*", "bin", name),
		filepath.Join(homeDir, ".npm-global", "bin", name),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		candidates = append(candidates, matches...)
	}
	for _, candidate := range candidates {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func resolveCommand(explicit, fallback string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	return strings.TrimSpace(fallback)
}

func fallbackString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func writeEnvFile(envFile string, values initValues) error {
	if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	if err := os.WriteFile(envFile, []byte(renderEnvFile(values)), 0o600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}
	return nil
}
