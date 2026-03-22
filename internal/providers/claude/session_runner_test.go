package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/vibegram/internal/runner"
)

func TestSessionRunnerStartParsesSessionIDAndMessage(t *testing.T) {
	cmd := writeFakeClaudeScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Start(context.Background(), workDir, "start prompt")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if result.ProviderSessionID != "claude-session-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "claude-session-123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
	if !strings.Contains(result.RawOutput, "\"session_id\":\"claude-session-123\"") {
		t.Fatalf("RawOutput = %q, want session_id", result.RawOutput)
	}
}

func TestSessionRunnerResumeReturnsLastMessage(t *testing.T) {
	cmd := writeFakeClaudeScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Resume(context.Background(), workDir, "claude-session-123", "follow-up prompt")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if result.ProviderSessionID != "claude-session-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "claude-session-123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
}

func writeFakeClaudeScript(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-claude.sh")
	script := `#!/bin/sh
echo '{"type":"result","subtype":"success","session_id":"claude-session-123","result":"assistant final message"}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
