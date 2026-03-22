package codex

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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

func TestSessionRunnerResumeDoesNotPassStartOnlyCdFlag(t *testing.T) {
	workDir := t.TempDir()
	fake := &capturingRunner{
		result: runner.Result{
			ExitCode: 0,
			Output:   `{"type":"thread.started","thread_id":"thread-123"}`,
		},
	}
	r := NewSessionRunner(fake, "/usr/local/bin/codex")

	result, err := r.Resume(context.Background(), workDir, "thread-123", "follow-up prompt")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if slices.Contains(fake.req.Args, "-C") {
		t.Fatalf("resume args = %v, do not want -C for codex exec resume", fake.req.Args)
	}
	if result.ProviderSessionID != "thread-123" {
		t.Fatalf("ProviderSessionID = %q, want %q", result.ProviderSessionID, "thread-123")
	}
}

type capturingRunner struct {
	req    runner.Request
	result runner.Result
	err    error
}

func (r *capturingRunner) Run(ctx context.Context, req runner.Request) (runner.Result, error) {
	r.req = req
	if outputFile, ok := outputFileFromArgs(req.Args); ok {
		if err := os.WriteFile(outputFile, []byte("assistant final message"), 0o644); err != nil {
			return runner.Result{}, err
		}
	}
	return r.result, r.err
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

func outputFileFromArgs(args []string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" || args[i] == "--output-last-message" {
			return args[i+1], true
		}
	}
	return "", false
}
