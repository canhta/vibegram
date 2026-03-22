package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type initValues struct {
	BotToken      string
	ForumChatID   string
	AdminIDs      string
	OperatorIDs   string
	CodexCommand  string
	ClaudeCommand string
	WorkRoot      string
	StateDir      string
	LogLevel      string
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string
	SystemPath    string
}

func runInit(ctx context.Context, stdin io.Reader, stdout io.Writer, envFile string, deps cliDeps) error {
	_ = ctx

	values, err := collectInitValues(stdin, stdout, deps)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}

	if err := os.WriteFile(envFile, []byte(renderEnvFile(values)), 0o600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Wrote config to %s\n", envFile)
	_, _ = fmt.Fprintf(stdout, "Next:\n")
	_, _ = fmt.Fprintf(stdout, "  sudo vibegram service install --env-file %s\n", envFile)
	_, _ = fmt.Fprintf(stdout, "  sudo vibegram service start\n")
	_, _ = fmt.Fprintf(stdout, "  sudo vibegram service status\n")
	return nil
}

func collectInitValues(stdin io.Reader, stdout io.Writer, deps cliDeps) (initValues, error) {
	reader := bufio.NewReader(stdin)
	codexDefault := detectedCommandPath(deps, "codex")
	if codexDefault == "" {
		codexDefault = "codex"
	}
	claudeDefault := detectedCommandPath(deps, "claude")
	if claudeDefault == "" {
		claudeDefault = "claude"
	}

	values := initValues{
		CodexCommand:  codexDefault,
		ClaudeCommand: claudeDefault,
		WorkRoot:      "/var/lib/vibegram",
		StateDir:      "/var/lib/vibegram/state",
		LogLevel:      "info",
	}

	var err error
	if values.BotToken, err = promptValue(reader, stdout, "Telegram bot token", ""); err != nil {
		return initValues{}, err
	}
	if values.ForumChatID, err = promptValue(reader, stdout, "Telegram forum chat ID", ""); err != nil {
		return initValues{}, err
	}
	if values.AdminIDs, err = promptValue(reader, stdout, "Admin Telegram user IDs (comma-separated, optional)", ""); err != nil {
		return initValues{}, err
	}
	if values.OperatorIDs, err = promptValue(reader, stdout, "Operator Telegram user IDs (comma-separated, optional)", ""); err != nil {
		return initValues{}, err
	}
	if values.CodexCommand, err = promptValue(reader, stdout, "Codex command", values.CodexCommand); err != nil {
		return initValues{}, err
	}
	if values.ClaudeCommand, err = promptValue(reader, stdout, "Claude command", values.ClaudeCommand); err != nil {
		return initValues{}, err
	}
	if values.WorkRoot, err = promptValue(reader, stdout, "Work root", values.WorkRoot); err != nil {
		return initValues{}, err
	}
	if values.StateDir, err = promptValue(reader, stdout, "State dir", values.StateDir); err != nil {
		return initValues{}, err
	}

	if strings.TrimSpace(values.BotToken) == "" {
		return initValues{}, fmt.Errorf("telegram bot token is required")
	}
	if strings.TrimSpace(values.ForumChatID) == "" {
		return initValues{}, fmt.Errorf("telegram forum chat ID is required")
	}

	return values, nil
}

func promptValue(reader *bufio.Reader, stdout io.Writer, label, fallback string) (string, error) {
	if fallback != "" {
		if _, err := fmt.Fprintf(stdout, "%s [%s]: ", label, fallback); err != nil {
			return "", err
		}
	} else {
		if _, err := fmt.Fprintf(stdout, "%s: ", label); err != nil {
			return "", err
		}
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = fallback
	}
	return value, nil
}

func detectedCommandPath(deps cliDeps, name string) string {
	if deps.lookPath == nil {
		return ""
	}
	path, err := deps.lookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func renderEnvFile(values initValues) string {
	lines := []string{
		"VIBEGRAM_TELEGRAM_BOT_TOKEN=" + values.BotToken,
		"VIBEGRAM_TELEGRAM_FORUM_CHAT_ID=" + values.ForumChatID,
	}

	if values.AdminIDs != "" {
		lines = append(lines, "VIBEGRAM_TELEGRAM_ADMIN_IDS="+values.AdminIDs)
	}
	if values.OperatorIDs != "" {
		lines = append(lines, "VIBEGRAM_TELEGRAM_OPERATOR_IDS="+values.OperatorIDs)
	}

	lines = append(lines, "")

	if values.CodexCommand != "" {
		lines = append(lines, "VIBEGRAM_PROVIDER_CODEX_CMD="+values.CodexCommand)
	}
	if values.ClaudeCommand != "" {
		lines = append(lines, "VIBEGRAM_PROVIDER_CLAUDE_CMD="+values.ClaudeCommand)
	}

	lines = append(lines, "")
	lines = append(lines, "VIBEGRAM_WORK_ROOT="+values.WorkRoot)
	lines = append(lines, "VIBEGRAM_STATE_DIR="+values.StateDir)
	lines = append(lines, "VIBEGRAM_LOG_LEVEL="+values.LogLevel)

	if values.OpenAIAPIKey != "" {
		lines = append(lines, "OPENAI_API_KEY="+values.OpenAIAPIKey)
	}
	if values.OpenAIBaseURL != "" {
		lines = append(lines, "VIBEGRAM_OPENAI_BASE_URL="+values.OpenAIBaseURL)
	}
	if values.OpenAIModel != "" {
		lines = append(lines, "VIBEGRAM_OPENAI_MODEL="+values.OpenAIModel)
	}
	if values.SystemPath != "" {
		lines = append(lines, "PATH="+values.SystemPath)
	}
	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

func buildServicePath(commands ...string) string {
	seen := make(map[string]struct{})
	dirs := make([]string, 0, len(commands)+1)
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" || !filepath.IsAbs(command) {
			continue
		}
		dir := filepath.Dir(command)
		if _, found := seen[dir]; found {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	dirs = append(dirs, "/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin")
	return strings.Join(dirs, ":")
}
