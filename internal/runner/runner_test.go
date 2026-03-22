package runner_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/runner"
)

func TestRunCapturesPTYOutput(t *testing.T) {
	r := runner.New()

	result, err := r.Run(context.Background(), runner.Request{
		CommandPath: "/bin/sh",
		Args:        []string{"-c", "printf 'hello from pty\\n'"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Output, "hello from pty") {
		t.Fatalf("Output = %q, want PTY output", result.Output)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunReportsNonZeroExitCode(t *testing.T) {
	r := runner.New()

	result, err := r.Run(context.Background(), runner.Request{
		CommandPath: "/bin/sh",
		Args:        []string{"-c", "printf 'boom\\n'; exit 3"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if result.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", result.ExitCode)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("Output = %q, want command output", result.Output)
	}
}

func TestRunStreamEmitsLinesBeforeProcessExit(t *testing.T) {
	r := runner.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstLine := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		_, err := r.RunStream(ctx, runner.Request{
			CommandPath: "/bin/sh",
			Args: []string{"-c", `
printf 'first line\n'
sleep 1
printf 'second line\n'
`},
		}, func(line string) {
			select {
			case firstLine <- line:
			default:
			}
		})
		done <- err
	}()

	select {
	case got := <-firstLine:
		if got != "first line" {
			t.Fatalf("first streamed line = %q, want %q", got, "first line")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive streamed line before process exit")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunStream() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunStream() did not finish")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	r := runner.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx, runner.Request{
			CommandPath: "/bin/sh",
			Args:        []string{"-c", "sleep 10"},
		})
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}
