package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/canhta/vibegram/internal/config"
	"github.com/creack/pty"
)

var ErrSandboxUnavailable = errors.New("sandbox enforcement unavailable")

type Runner struct{}

type Request struct {
	CommandPath    string
	Args           []string
	Dir            string
	Env            []string
	SandboxProfile config.SandboxProfile
}

type Result struct {
	Output         string
	ExitCode       int
	SandboxProfile config.SandboxProfile
}

func New() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	if req.CommandPath == "" {
		return Result{}, fmt.Errorf("command path is required")
	}
	if !filepath.IsAbs(req.CommandPath) {
		return Result{}, fmt.Errorf("command path must be absolute")
	}

	effectiveProfile := req.SandboxProfile
	if effectiveProfile == "" {
		effectiveProfile = config.SandboxProfileWorkspaceWriteNetworkOff
	}
	if effectiveProfile != config.SandboxProfileFullAccess {
		return Result{
			ExitCode:       -1,
			SandboxProfile: effectiveProfile,
		}, fmt.Errorf("%w for profile %q", ErrSandboxUnavailable, effectiveProfile)
	}

	cmd := exec.CommandContext(ctx, req.CommandPath, req.Args...)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), req.Env...)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return Result{}, fmt.Errorf("start pty command: %w", err)
	}

	var output bytes.Buffer
	copyErrCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(&output, ptmx)
		copyErrCh <- err
	}()

	waitErr := cmd.Wait()
	_ = ptmx.Close()
	copyErr := <-copyErrCh

	if copyErr != nil && !errors.Is(copyErr, os.ErrClosed) && !errors.Is(copyErr, syscall.EIO) {
		return Result{}, fmt.Errorf("capture pty output: %w", copyErr)
	}

	result := Result{
		Output:         output.String(),
		ExitCode:       exitCode(cmd.ProcessState, waitErr),
		SandboxProfile: effectiveProfile,
	}

	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, fmt.Errorf("wait for command: %w", waitErr)
	}

	return result, nil
}

func exitCode(state *os.ProcessState, waitErr error) int {
	if state != nil {
		return state.ExitCode()
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}
