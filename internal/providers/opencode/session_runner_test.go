package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/vibegram/internal/runner"
)

func TestSessionRunnerStartParsesSessionIDAndMessage(t *testing.T) {
	cmd := writeFakeOpencodeScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Start(context.Background(), workDir, "start prompt")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if result.ProviderSessionID != "ses_123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "ses_123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
}

func TestSessionRunnerResumeReturnsLastMessage(t *testing.T) {
	cmd := writeFakeOpencodeScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Resume(context.Background(), workDir, "ses_123", "follow-up prompt")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if result.ProviderSessionID != "ses_123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "ses_123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
}

func writeFakeOpencodeScript(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-opencode.sh")
	script := `#!/bin/sh
echo '{"type":"step_start","sessionID":"ses_123","part":{"type":"step-start"}}'
echo '{"type":"text","sessionID":"ses_123","part":{"type":"text","text":"assistant final message"}}'
echo '{"type":"step_finish","sessionID":"ses_123","part":{"type":"step-finish"}}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
