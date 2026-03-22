package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const defaultServiceUser = "vibegram"

func runService(ctx context.Context, args []string, stdout, stderr io.Writer, deps cliDeps) error {
	if len(args) == 0 {
		return fmt.Errorf("service subcommand is required")
	}

	switch args[0] {
	case "print":
		fs := flag.NewFlagSet("service print", flag.ContinueOnError)
		fs.SetOutput(stderr)
		envFile := fs.String("env-file", "/etc/vibegram/env", "path to vibegram env file")
		workRoot := fs.String("work-root", "/var/lib/vibegram", "service work root")
		serviceUser := fs.String("user", defaultServiceUser, "service account")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		executable, err := deps.executablePath()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
		_, _ = io.WriteString(stdout, renderSystemdUnit(executable, *envFile, *workRoot, *serviceUser))
		return nil
	case "install":
		fs := flag.NewFlagSet("service install", flag.ContinueOnError)
		fs.SetOutput(stderr)
		envFile := fs.String("env-file", "/etc/vibegram/env", "path to vibegram env file")
		unitFile := fs.String("unit-file", "/etc/systemd/system/vibegram.service", "path to systemd unit file")
		workRoot := fs.String("work-root", "/var/lib/vibegram", "service work root")
		serviceUser := fs.String("user", defaultServiceUser, "service account")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return installService(ctx, stdout, deps, *envFile, *unitFile, *workRoot, *serviceUser)
	case "start":
		return deps.runCommand(ctx, "systemctl", "enable", "--now", "vibegram")
	case "stop":
		return deps.runCommand(ctx, "systemctl", "stop", "vibegram")
	case "status":
		return deps.runCommand(ctx, "systemctl", "status", "vibegram", "--no-pager")
	case "logs":
		return deps.runCommand(ctx, "journalctl", "-u", "vibegram", "-n", "200", "--no-pager")
	default:
		return fmt.Errorf("unknown service subcommand %q", args[0])
	}
}

func installService(ctx context.Context, stdout io.Writer, deps cliDeps, envFile, unitFile, workRoot, serviceUser string) error {
	if _, err := os.Stat(envFile); err != nil {
		return fmt.Errorf("env file %s not found; run `vibegram init --env-file %s` first", envFile, envFile)
	}

	if _, err := deps.lookupUser(serviceUser); err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			if err := deps.runCommand(ctx, "useradd", "--system", "--home", workRoot, "--shell", "/usr/sbin/nologin", serviceUser); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("lookup user %s: %w", serviceUser, err)
		}
	}

	if err := os.MkdirAll(filepath.Join(workRoot, "state"), 0o755); err != nil {
		return fmt.Errorf("create work root: %w", err)
	}
	if err := deps.runCommand(ctx, "chown", "-R", serviceUser+":"+serviceUser, workRoot); err != nil {
		return err
	}

	executable, err := deps.executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(unitFile), 0o755); err != nil {
		return fmt.Errorf("create unit dir: %w", err)
	}
	if err := os.WriteFile(unitFile, []byte(renderSystemdUnit(executable, envFile, workRoot, serviceUser)), 0o644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	if err := deps.runCommand(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "Installed %s\n", unitFile)
	_, _ = fmt.Fprintf(stdout, "Next:\n")
	_, _ = fmt.Fprintf(stdout, "  sudo systemctl enable --now vibegram\n")
	_, _ = fmt.Fprintf(stdout, "  sudo systemctl status vibegram --no-pager\n")
	_, _ = fmt.Fprintf(stdout, "  sudo journalctl -u vibegram -n 200 --no-pager\n")
	return nil
}

func renderSystemdUnit(executable, envFile, workRoot, serviceUser string) string {
	stateDir := filepath.Join(workRoot, "state")
	lines := []string{
		"[Unit]",
		"Description=vibegram - Telegram supervision layer for coding agents",
		"After=network.target",
		"",
		"[Service]",
		"Type=simple",
		"User=" + serviceUser,
		"StateDirectory=" + filepath.Base(workRoot),
		"WorkingDirectory=" + workRoot,
		"Environment=VIBEGRAM_WORK_ROOT=" + workRoot,
		"Environment=VIBEGRAM_STATE_DIR=" + stateDir,
		"EnvironmentFile=" + envFile,
		"ExecStart=" + strings.Join([]string{executable, "daemon", "--env-file", envFile}, " "),
		"Restart=on-failure",
		"RestartSec=5s",
		"StandardOutput=journal",
		"StandardError=journal",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}
	return strings.Join(lines, "\n")
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := execCommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
