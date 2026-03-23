package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testUser(name, home, gid string) *user.User {
	return &user.User{
		Username: name,
		HomeDir:  home,
		Gid:      gid,
	}
}

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
	deps.lookupUser = func(name string) (*user.User, error) {
		return testUser("ubuntu", "/home/ubuntu", "1001"), nil
	}

	if err := runArgsWithDeps(context.Background(), []string{"service", "print", "--env-file", "/etc/vibegram/env", "--user", "ubuntu"}, strings.NewReader(""), stdout, new(bytes.Buffer), deps); err != nil {
		t.Fatalf("runArgsWithDeps(service print) error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "EnvironmentFile=/etc/vibegram/env") {
		t.Fatalf("output = %q, want env file", output)
	}
	if !strings.Contains(output, "Environment=HOME=/home/ubuntu") {
		t.Fatalf("output = %q, want service home", output)
	}
	if !strings.Contains(output, "ExecStart=/usr/local/bin/vibegram daemon --env-file /etc/vibegram/env") {
		t.Fatalf("output = %q, want daemon exec start", output)
	}
}

func TestRunArgsServicePrintPrefersOperatorAccountByDefault(t *testing.T) {
	stdout := new(bytes.Buffer)
	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return "/usr/local/bin/vibegram", nil
	}
	deps.lookupUser = func(name string) (*user.User, error) {
		if name != "ubuntu" {
			t.Fatalf("lookupUser(%q)", name)
		}
		return testUser("ubuntu", "/home/ubuntu", "1001"), nil
	}
	deps.currentUser = func() (*user.User, error) {
		return testUser("root", "/root", "0"), nil
	}
	deps.getenv = func(key string) string {
		if key == "SUDO_USER" {
			return "ubuntu"
		}
		return ""
	}

	if err := runArgsWithDeps(context.Background(), []string{"service", "print", "--env-file", "/etc/vibegram/env"}, strings.NewReader(""), stdout, new(bytes.Buffer), deps); err != nil {
		t.Fatalf("runArgsWithDeps(service print) error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "User=ubuntu") {
		t.Fatalf("output = %q, want operator service user", output)
	}
	if !strings.Contains(output, "Environment=HOME=/home/ubuntu") {
		t.Fatalf("output = %q, want operator home", output)
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
	deps.currentUser = func() (*user.User, error) {
		return testUser("root", "/root", "0"), nil
	}
	deps.getenv = func(key string) string { return "" }
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
		"chown root:vibegram " + envPath,
		"chmod 640 " + envPath,
		"systemctl daemon-reload",
	} {
		if !containsString(calls, want) {
			t.Fatalf("command calls = %v, want %q", calls, want)
		}
	}

	if !strings.Contains(unitText, "Environment=HOME="+workRoot) {
		t.Fatalf("unit file = %q, want service home", unitText)
	}

	if !strings.Contains(stdout.String(), "systemctl enable --now vibegram") {
		t.Fatalf("stdout = %q, want start hint", stdout.String())
	}
}

func TestRunArgsServiceInstallPrefersOperatorAccountByDefault(t *testing.T) {
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
		if name != "ubuntu" {
			t.Fatalf("lookupUser(%q)", name)
		}
		return testUser("ubuntu", "/home/ubuntu", "1001"), nil
	}
	deps.currentUser = func() (*user.User, error) {
		return testUser("root", "/root", "0"), nil
	}
	deps.getenv = func(key string) string {
		if key == "SUDO_USER" {
			return "ubuntu"
		}
		return ""
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
	if !strings.Contains(unitText, "User=ubuntu") {
		t.Fatalf("unit file = %q, want operator service user", unitText)
	}
	if !strings.Contains(unitText, "Environment=HOME=/home/ubuntu") {
		t.Fatalf("unit file = %q, want operator home", unitText)
	}

	for _, want := range []string{
		"chown -R ubuntu:1001 " + workRoot,
		"chown root:1001 " + envPath,
		"chmod 640 " + envPath,
		"systemctl daemon-reload",
	} {
		if !containsString(calls, want) {
			t.Fatalf("command calls = %v, want %q", calls, want)
		}
	}
}

func TestRunArgsUpgradeHelpIsRecognized(t *testing.T) {
	err := runArgsWithDeps(
		context.Background(),
		[]string{"upgrade", "-h"},
		strings.NewReader(""),
		new(bytes.Buffer),
		new(bytes.Buffer),
		defaultCLIDeps(),
	)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("runArgsWithDeps(upgrade -h) error = %v, want flag.ErrHelp", err)
	}
}

func TestRunArgsUpgradeDownloadsLatestReleaseAndRestartsService(t *testing.T) {
	tmp := t.TempDir()
	currentBinaryPath := filepath.Join(tmp, "usr", "local", "bin", "vibegram")
	if err := os.MkdirAll(filepath.Dir(currentBinaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(currentBinaryPath), err)
	}

	oldBinary := []byte("old-binary")
	if err := os.WriteFile(currentBinaryPath, oldBinary, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", currentBinaryPath, err)
	}

	version := "v0.1.0"
	assetVersion := strings.TrimPrefix(version, "v")
	assetBase := fmt.Sprintf("vibegram_%s_%s_%s", assetVersion, runtime.GOOS, runtime.GOARCH)
	tarballName := assetBase + ".tar.gz"
	newBinary := []byte("new-binary")
	tarball := releaseTarball(t, assetBase, newBinary)
	checksum := sha256Hex(tarball)

	var fetched []string
	var commands []string
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return currentBinaryPath, nil
	}
	deps.fetchURL = func(ctx context.Context, url string) ([]byte, error) {
		fetched = append(fetched, url)
		switch url {
		case "https://api.github.com/repos/canhta/vibegram/releases/latest":
			return []byte(`{"tag_name":"v0.1.0"}`), nil
		case "https://github.com/canhta/vibegram/releases/download/v0.1.0/SHA256SUMS":
			return []byte(checksum + "  ./" + tarballName + "\n"), nil
		case "https://github.com/canhta/vibegram/releases/download/v0.1.0/" + tarballName:
			return tarball, nil
		default:
			return nil, fmt.Errorf("unexpected URL %q", url)
		}
	}
	deps.runCommand = func(ctx context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := runArgsWithDeps(context.Background(), []string{"upgrade"}, strings.NewReader(""), stdout, stderr, deps); err != nil {
		t.Fatalf("runArgsWithDeps(upgrade) error = %v", err)
	}

	gotBinary, err := os.ReadFile(currentBinaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", currentBinaryPath, err)
	}
	if string(gotBinary) != string(newBinary) {
		t.Fatalf("installed binary = %q, want %q", gotBinary, newBinary)
	}

	backupMatches, err := filepath.Glob(currentBinaryPath + ".bak-*")
	if err != nil {
		t.Fatalf("Glob(backup) error = %v", err)
	}
	if len(backupMatches) != 1 {
		t.Fatalf("backup files = %v, want exactly one", backupMatches)
	}
	backupBinary, err := os.ReadFile(backupMatches[0])
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", backupMatches[0], err)
	}
	if string(backupBinary) != string(oldBinary) {
		t.Fatalf("backup binary = %q, want %q", backupBinary, oldBinary)
	}

	for _, want := range []string{
		"https://api.github.com/repos/canhta/vibegram/releases/latest",
		"https://github.com/canhta/vibegram/releases/download/v0.1.0/SHA256SUMS",
		"https://github.com/canhta/vibegram/releases/download/v0.1.0/" + tarballName,
	} {
		if !containsString(fetched, want) {
			t.Fatalf("fetched URLs = %v, want %q", fetched, want)
		}
	}

	for _, want := range []string{
		"systemctl restart vibegram",
		"systemctl status vibegram --no-pager",
	} {
		if !containsString(commands, want) {
			t.Fatalf("command calls = %v, want %q", commands, want)
		}
	}

	if !strings.Contains(stdout.String(), "Upgraded vibegram to v0.1.0") {
		t.Fatalf("stdout = %q, want upgrade confirmation", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunArgsUpgradeRejectsChecksumMismatch(t *testing.T) {
	tmp := t.TempDir()
	currentBinaryPath := filepath.Join(tmp, "usr", "local", "bin", "vibegram")
	if err := os.MkdirAll(filepath.Dir(currentBinaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(currentBinaryPath), err)
	}

	oldBinary := []byte("old-binary")
	if err := os.WriteFile(currentBinaryPath, oldBinary, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", currentBinaryPath, err)
	}

	version := "v0.1.0"
	assetVersion := strings.TrimPrefix(version, "v")
	assetBase := fmt.Sprintf("vibegram_%s_%s_%s", assetVersion, runtime.GOOS, runtime.GOARCH)
	tarballName := assetBase + ".tar.gz"
	tarball := releaseTarball(t, assetBase, []byte("new-binary"))

	var commands []string
	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return currentBinaryPath, nil
	}
	deps.fetchURL = func(ctx context.Context, url string) ([]byte, error) {
		switch url {
		case "https://github.com/canhta/vibegram/releases/download/v0.1.0/SHA256SUMS":
			return []byte(strings.Repeat("0", 64) + "  ./" + tarballName + "\n"), nil
		case "https://github.com/canhta/vibegram/releases/download/v0.1.0/" + tarballName:
			return tarball, nil
		default:
			return nil, fmt.Errorf("unexpected URL %q", url)
		}
	}
	deps.runCommand = func(ctx context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil
	}

	err := runArgsWithDeps(
		context.Background(),
		[]string{"upgrade", "--version", version},
		strings.NewReader(""),
		new(bytes.Buffer),
		new(bytes.Buffer),
		deps,
	)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("runArgsWithDeps(upgrade --version %s) error = %v, want checksum mismatch", version, err)
	}

	gotBinary, err := os.ReadFile(currentBinaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", currentBinaryPath, err)
	}
	if string(gotBinary) != string(oldBinary) {
		t.Fatalf("installed binary = %q, want original %q", gotBinary, oldBinary)
	}
	if len(commands) != 0 {
		t.Fatalf("command calls = %v, want none", commands)
	}

	backupMatches, err := filepath.Glob(currentBinaryPath + ".bak-*")
	if err != nil {
		t.Fatalf("Glob(backup) error = %v", err)
	}
	if len(backupMatches) != 0 {
		t.Fatalf("backup files = %v, want none", backupMatches)
	}
}

func TestRunArgsInstallBootstrapsServiceInOneCommand(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "etc", "vibegram", "env")
	unitPath := filepath.Join(tmp, "etc", "systemd", "system", "vibegram.service")
	workRoot := filepath.Join(tmp, "var", "lib", "vibegram")

	var calls []string
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	deps := defaultCLIDeps()
	deps.executablePath = func() (string, error) {
		return "/usr/local/bin/vibegram", nil
	}
	deps.lookupUser = func(name string) (*user.User, error) {
		if name != "ubuntu" {
			t.Fatalf("lookupUser(%q)", name)
		}
		return testUser("ubuntu", "/home/ubuntu", "1001"), nil
	}
	deps.runCommand = func(ctx context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}
	deps.currentUser = func() (*user.User, error) {
		return testUser("root", "/root", "0"), nil
	}
	deps.getenv = func(key string) string {
		if key == "SUDO_USER" {
			return "ubuntu"
		}
		return ""
	}

	err := runArgsWithDeps(
		context.Background(),
		[]string{
			"install",
			"--env-file", envPath,
			"--unit-file", unitPath,
			"--work-root", workRoot,
			"--bot-token", "telegram-token",
			"--chat-id", "-1001234567890",
			"--admin-ids", "1001",
			"--operator-ids", "1002",
			"--codex-cmd", "/home/ubuntu/.nvm/versions/node/v25.8.1/bin/codex",
			"--claude-cmd", "/home/ubuntu/.local/bin/claude",
		},
		strings.NewReader(""),
		stdout,
		stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("runArgsWithDeps(install) error = %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", envPath, err)
	}
	text := string(data)
	for _, want := range []string{
		"VIBEGRAM_TELEGRAM_BOT_TOKEN=telegram-token",
		"VIBEGRAM_TELEGRAM_FORUM_CHAT_ID=-1001234567890",
		"VIBEGRAM_TELEGRAM_ADMIN_IDS=1001",
		"VIBEGRAM_TELEGRAM_OPERATOR_IDS=1002",
		"VIBEGRAM_PROVIDER_CODEX_CMD=/home/ubuntu/.nvm/versions/node/v25.8.1/bin/codex",
		"VIBEGRAM_PROVIDER_CLAUDE_CMD=/home/ubuntu/.local/bin/claude",
		"VIBEGRAM_WORK_ROOT=" + workRoot,
		"VIBEGRAM_STATE_DIR=" + filepath.Join(workRoot, "state"),
		"PATH=/home/ubuntu/.local/bin:/home/ubuntu/.nvm/versions/node/v25.8.1/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q\n%s", want, text)
		}
	}

	unitData, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", unitPath, err)
	}
	unitText := string(unitData)
	if !strings.Contains(unitText, "User=ubuntu") {
		t.Fatalf("unit file = %q, want service user", unitText)
	}
	if !strings.Contains(unitText, "Environment=HOME=/home/ubuntu") {
		t.Fatalf("unit file = %q, want service home", unitText)
	}

	for _, want := range []string{
		"chown root:1001 " + envPath,
		"chmod 640 " + envPath,
		"systemctl daemon-reload",
		"systemctl enable --now vibegram",
		"systemctl status vibegram --no-pager",
	} {
		if !containsString(calls, want) {
			t.Fatalf("command calls = %v, want %q", calls, want)
		}
	}

	if !strings.Contains(stdout.String(), "Installed "+unitPath) {
		t.Fatalf("stdout = %q, want install output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
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

func releaseTarball(t *testing.T, root string, binary []byte) []byte {
	t.Helper()

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)

	writeTarEntry := func(name string, mode int64, data []byte) {
		t.Helper()
		hdr := &tar.Header{
			Name: name,
			Mode: mode,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}

	if err := tw.WriteHeader(&tar.Header{Name: root + "/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatalf("WriteHeader(root) error = %v", err)
	}
	writeTarEntry(root+"/README.md", 0o644, []byte("README"))
	writeTarEntry(root+"/vibegram", 0o755, binary)

	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar writer error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("Close gzip writer error = %v", err)
	}

	return archive.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
