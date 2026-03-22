package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	return r.run(ctx, workDir, []string{"exec", "--json", "--skip-git-repo-check", "-C", workDir, prompt}, true)
}

func (r *SessionRunner) Resume(ctx context.Context, workDir, providerSessionID, prompt string) (SessionResult, error) {
	result, err := r.run(ctx, workDir, []string{"exec", "resume", providerSessionID, "--json", "--skip-git-repo-check", "-C", workDir, prompt}, false)
	if err != nil {
		return SessionResult{}, err
	}
	result.ProviderSessionID = providerSessionID
	return result, nil
}

func (r *SessionRunner) run(ctx context.Context, workDir string, args []string, parseThreadID bool) (SessionResult, error) {
	outputFile, err := r.tempOutputFile()
	if err != nil {
		return SessionResult{}, err
	}
	defer os.Remove(outputFile)

	args = append(args[:len(args)-1], append([]string{"-o", outputFile}, args[len(args)-1])...)

	res, err := r.runner.Run(ctx, runner.Request{
		CommandPath: r.commandPath,
		Args:        args,
		Dir:         workDir,
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("run codex: %w", err)
	}
	if res.ExitCode != 0 {
		return SessionResult{}, fmt.Errorf("codex exit %d: %s", res.ExitCode, strings.TrimSpace(res.Output))
	}

	message, err := readTrimmed(outputFile)
	if err != nil {
		return SessionResult{}, err
	}

	result := SessionResult{
		Message:   message,
		RawOutput: res.Output,
	}
	if parseThreadID {
		threadID, err := parseThreadIDFromOutput(res.Output)
		if err != nil {
			return SessionResult{}, err
		}
		result.ProviderSessionID = threadID
	}

	return result, nil
}

func (r *SessionRunner) tempOutputFile() (string, error) {
	file, err := os.CreateTemp("", "vibegram-codex-last-message-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp output file: %w", err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temp output file: %w", err)
	}
	return name, nil
}

func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func parseThreadIDFromOutput(output string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			return event.ThreadID, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan codex output: %w", err)
	}

	return "", fmt.Errorf("codex output missing thread.started event")
}
