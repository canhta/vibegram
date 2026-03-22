package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/vibegram/internal/runner"
)

func TestSessionRunnerStartParsesThreadIDAndMessage(t *testing.T) {
	cmd := writeFakeCodexScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Start(context.Background(), workDir, "start prompt")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if result.ProviderSessionID != "thread-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "thread-123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
	if !strings.Contains(result.RawOutput, "\"thread_id\":\"thread-123\"") {
		t.Fatalf("RawOutput = %q, want thread.started line", result.RawOutput)
	}
}

func TestSessionRunnerResumeReturnsLastMessage(t *testing.T) {
	cmd := writeFakeCodexScript(t)
	workDir := t.TempDir()
	r := NewSessionRunner(runner.New(), cmd)

	result, err := r.Resume(context.Background(), workDir, "thread-123", "follow-up prompt")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if result.ProviderSessionID != "thread-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "thread-123")
	}
	if result.Message != "assistant final message" {
		t.Fatalf("Message = %q, want %q", result.Message, "assistant final message")
	}
}

func writeFakeCodexScript(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-codex.sh")
	script := `#!/bin/sh
output_file=""
mode=""
resume_id=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then
    output_file="$arg"
  fi
  if [ "$arg" = "resume" ]; then
    mode="resume"
  elif [ "$mode" = "resume" ] && [ -z "$resume_id" ] && [ "$arg" != "--json" ]; then
    resume_id="$arg"
  fi
  prev="$arg"
done

echo '{"type":"thread.started","thread_id":"thread-123"}'
if [ -n "$output_file" ]; then
  printf 'assistant final message' > "$output_file"
fi
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
