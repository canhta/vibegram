package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/vibegram/internal/providers"
	"github.com/canhta/vibegram/internal/runner"
)

type commandRunner interface {
	Run(ctx context.Context, req runner.Request) (runner.Result, error)
}

type SessionResult = providers.SessionResult

type SessionRunner struct {
	runner      commandRunner
	commandPath string
}

func NewSessionRunner(r commandRunner, commandPath string) *SessionRunner {
	return &SessionRunner{
		runner:      r,
		commandPath: commandPath,
	}
}

func (r *SessionRunner) Start(ctx context.Context, workDir, prompt string) (SessionResult, error) {
	return r.run(ctx, workDir, []string{
		"--dangerously-skip-permissions",
		"--add-dir", workDir,
		"-p",
		"--output-format", "json",
		prompt,
	}, true)
}

func (r *SessionRunner) Resume(ctx context.Context, workDir, providerSessionID, prompt string) (SessionResult, error) {
	result, err := r.run(ctx, workDir, []string{
		"--dangerously-skip-permissions",
		"--add-dir", workDir,
		"-p",
		"--output-format", "json",
		"-r", providerSessionID,
		prompt,
	}, false)
	if err != nil {
		return SessionResult{}, err
	}
	if strings.TrimSpace(result.ProviderSessionID) == "" {
		result.ProviderSessionID = providerSessionID
	}
	return result, nil
}

func (r *SessionRunner) run(ctx context.Context, workDir string, args []string, requireSessionID bool) (SessionResult, error) {
	res, err := r.runner.Run(ctx, runner.Request{
		CommandPath: r.commandPath,
		Args:        args,
		Dir:         workDir,
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("run claude: %w", err)
	}
	if res.ExitCode != 0 {
		return SessionResult{}, fmt.Errorf("claude exit %d: %s", res.ExitCode, strings.TrimSpace(res.Output))
	}

	sessionID, message, err := parseResultOutput(res.Output)
	if err != nil {
		return SessionResult{}, err
	}
	if requireSessionID && strings.TrimSpace(sessionID) == "" {
		return SessionResult{}, fmt.Errorf("claude output missing session id")
	}

	return SessionResult{
		ProviderSessionID: sessionID,
		Message:           message,
		RawOutput:         res.Output,
	}, nil
}

func parseResultOutput(output string) (string, string, error) {
	var sessionID string
	var message string

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var event struct {
			Type      string `json:"type"`
			Subtype   string `json:"subtype"`
			SessionID string `json:"session_id"`
			Result    string `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "result" || event.Subtype != "success" {
			continue
		}
		if strings.TrimSpace(event.SessionID) != "" {
			sessionID = strings.TrimSpace(event.SessionID)
		}
		if strings.TrimSpace(event.Result) != "" {
			message = strings.TrimSpace(event.Result)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("scan claude output: %w", err)
	}
	if message == "" {
		return "", "", fmt.Errorf("claude output missing final text message")
	}

	return sessionID, message, nil
}
