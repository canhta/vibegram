package opencode

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
	return &SessionRunner{runner: r, commandPath: commandPath}
}

func (r *SessionRunner) Start(ctx context.Context, workDir, prompt string) (SessionResult, error) {
	return r.run(ctx, workDir, []string{"run", "--format", "json", "--dir", workDir, prompt}, true)
}

func (r *SessionRunner) Resume(ctx context.Context, workDir, providerSessionID, prompt string) (SessionResult, error) {
	result, err := r.run(ctx, workDir, []string{"run", "--format", "json", "--session", providerSessionID, "--dir", workDir, prompt}, false)
	if err != nil {
		return SessionResult{}, err
	}
	result.ProviderSessionID = providerSessionID
	return result, nil
}

func (r *SessionRunner) run(ctx context.Context, workDir string, args []string, parseSessionID bool) (SessionResult, error) {
	res, err := r.runner.Run(ctx, runner.Request{
		CommandPath: r.commandPath,
		Args:        args,
		Dir:         workDir,
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("run opencode: %w", err)
	}
	if res.ExitCode != 0 {
		return SessionResult{}, fmt.Errorf("opencode exit %d: %s", res.ExitCode, strings.TrimSpace(res.Output))
	}

	sessionID, message, err := parseOutput(res.Output)
	if err != nil {
		return SessionResult{}, err
	}

	result := SessionResult{
		Message:   message,
		RawOutput: res.Output,
	}
	if parseSessionID {
		result.ProviderSessionID = sessionID
	}
	return result, nil
}

func parseOutput(output string) (string, string, error) {
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
			SessionID string `json:"sessionID"`
			Part      struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.SessionID != "" && sessionID == "" {
			sessionID = event.SessionID
		}
		if event.Type == "text" && strings.TrimSpace(event.Part.Text) != "" {
			message = strings.TrimSpace(event.Part.Text)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("scan opencode output: %w", err)
	}
	if sessionID == "" {
		return "", "", fmt.Errorf("opencode output missing session id")
	}
	if message == "" {
		return "", "", fmt.Errorf("opencode output missing final text message")
	}
	return sessionID, message, nil
}
