package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunArgsInitWritesEnvFileAndPrintsNextSteps(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "vibegram.env")

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	input := strings.NewReader(strings.Join([]string{
		"telegram-token",
		"-1001234567890",
		"1001,1002",
		"",
		"",
		"",
		"",
		"",
	}, "\n"))

	deps := defaultCLIDeps()
	deps.lookPath = func(name string) (string, error) {
		switch name {
		case "codex":
			return "/usr/local/bin/codex", nil
		case "claude":
			return "/usr/local/bin/claude", nil
		default:
			return "", errors.New("not found")
		}
	}

	if err := runArgsWithDeps(context.Background(), []string{"init", "--env-file", envPath}, input, stdout, stderr, deps); err != nil {
		t.Fatalf("runArgsWithDeps(init) error = %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", envPath, err)
	}

	text := string(data)
	for _, want := range []string{
		"VIBEGRAM_TELEGRAM_BOT_TOKEN=telegram-token",
		"VIBEGRAM_TELEGRAM_FORUM_CHAT_ID=-1001234567890",
		"VIBEGRAM_TELEGRAM_ADMIN_IDS=1001,1002",
		"VIBEGRAM_PROVIDER_CODEX_CMD=/usr/local/bin/codex",
		"VIBEGRAM_PROVIDER_CLAUDE_CMD=/usr/local/bin/claude",
		"VIBEGRAM_WORK_ROOT=/var/lib/vibegram",
		"VIBEGRAM_STATE_DIR=/var/lib/vibegram/state",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q\n%s", want, text)
		}
	}

	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", envPath, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("env file mode = %o, want 600", got)
	}

	if !strings.Contains(stdout.String(), "vibegram service install") {
		t.Fatalf("stdout = %q, want install hint", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunArgsServicePrintRendersSystemdUnit(t *testing.T) {
	stdout := new(bytes.Buffer)
	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return "/usr/local/bin/vibegram", nil
	}

	if err := runArgsWithDeps(context.Background(), []string{"service", "print", "--env-file", "/etc/vibegram/env"}, strings.NewReader(""), stdout, new(bytes.Buffer), deps); err != nil {
		t.Fatalf("runArgsWithDeps(service print) error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "EnvironmentFile=/etc/vibegram/env") {
		t.Fatalf("output = %q, want env file", output)
	}
	if !strings.Contains(output, "ExecStart=/usr/local/bin/vibegram daemon --env-file /etc/vibegram/env") {
		t.Fatalf("output = %q, want daemon exec start", output)
	}
}

func TestRunArgsServiceInstallWritesUnitAndPreparesSystemd(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "env")
	unitPath := filepath.Join(tmp, "vibegram.service")
	workRoot := filepath.Join(tmp, "var", "lib", "vibegram")

	if err := os.WriteFile(envPath, []byte("VIBEGRAM_TELEGRAM_BOT_TOKEN=token\nVIBEGRAM_TELEGRAM_FORUM_CHAT_ID=-1001234567890\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", envPath, err)
	}

	var calls []string
	stdout := new(bytes.Buffer)
	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return "/usr/local/bin/vibegram", nil
	}
	deps.lookupUser = func(name string) (*user.User, error) {
		return nil, user.UnknownUserError(name)
	}
	deps.runCommand = func(ctx context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	err := runArgsWithDeps(
		context.Background(),
		[]string{"service", "install", "--env-file", envPath, "--unit-file", unitPath, "--work-root", workRoot},
		strings.NewReader(""),
		stdout,
		new(bytes.Buffer),
		deps,
	)
	if err != nil {
		t.Fatalf("runArgsWithDeps(service install) error = %v", err)
	}

	unitData, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", unitPath, err)
	}
	unitText := string(unitData)
	if !strings.Contains(unitText, "ExecStart=/usr/local/bin/vibegram daemon --env-file "+envPath) {
		t.Fatalf("unit file = %q, want daemon exec", unitText)
	}

	if _, err := os.Stat(filepath.Join(workRoot, "state")); err != nil {
		t.Fatalf("Stat(state dir) error = %v", err)
	}

	for _, want := range []string{
		"useradd --system --home " + workRoot + " --shell /usr/sbin/nologin vibegram",
		"chown -R vibegram:vibegram " + workRoot,
		"systemctl daemon-reload",
	} {
		if !containsString(calls, want) {
			t.Fatalf("command calls = %v, want %q", calls, want)
		}
	}

	if !strings.Contains(stdout.String(), "systemctl enable --now vibegram") {
		t.Fatalf("stdout = %q, want start hint", stdout.String())
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
