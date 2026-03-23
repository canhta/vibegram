package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	latestReleaseAPIURL = "https://api.github.com/repos/canhta/vibegram/releases/latest"
	releaseDownloadURL  = "https://github.com/canhta/vibegram/releases/download"
)

func runUpgrade(ctx context.Context, args []string, stdout, stderr io.Writer, deps cliDeps) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(stderr)

	version := fs.String("version", "", "release tag to install, for example v0.1.0")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedVersion, err := resolveUpgradeVersion(ctx, strings.TrimSpace(*version), deps)
	if err != nil {
		return err
	}

	executablePath, err := deps.executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	assetBase, tarballName := releaseAssetNames(resolvedVersion)
	checksumURL := releaseDownloadURL + "/" + resolvedVersion + "/SHA256SUMS"
	tarballURL := releaseDownloadURL + "/" + resolvedVersion + "/" + tarballName

	checksums, err := deps.fetchURL(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	tarball, err := deps.fetchURL(ctx, tarballURL)
	if err != nil {
		return fmt.Errorf("download release tarball: %w", err)
	}
	if err := verifyReleaseChecksum(checksums, tarballName, tarball); err != nil {
		return err
	}

	binary, mode, err := extractBinaryFromTarball(tarball, assetBase)
	if err != nil {
		return err
	}

	backupPath, err := backupAndInstallExecutable(executablePath, binary, mode)
	if err != nil {
		return err
	}

	if err := deps.runCommand(ctx, "systemctl", "restart", "vibegram"); err != nil {
		return fmt.Errorf("restart vibegram after upgrade (backup at %s): %w", backupPath, err)
	}
	if err := deps.runCommand(ctx, "systemctl", "status", "vibegram", "--no-pager"); err != nil {
		return fmt.Errorf("check vibegram status after upgrade (backup at %s): %w", backupPath, err)
	}

	_, _ = fmt.Fprintf(stdout, "Upgraded vibegram to %s\n", resolvedVersion)
	_, _ = fmt.Fprintf(stdout, "Installed %s\n", executablePath)
	_, _ = fmt.Fprintf(stdout, "Backup saved to %s\n", backupPath)
	return nil
}

func resolveUpgradeVersion(ctx context.Context, requested string, deps cliDeps) (string, error) {
	if requested != "" {
		return normalizeReleaseTag(requested), nil
	}

	body, err := deps.fetchURL(ctx, latestReleaseAPIURL)
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode latest release metadata: %w", err)
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("latest release metadata did not include tag_name")
	}
	return normalizeReleaseTag(payload.TagName), nil
}

func normalizeReleaseTag(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func releaseAssetNames(version string) (string, string) {
	assetVersion := strings.TrimPrefix(normalizeReleaseTag(version), "v")
	base := fmt.Sprintf("vibegram_%s_%s_%s", assetVersion, runtime.GOOS, runtime.GOARCH)
	return base, base + ".tar.gz"
}

func verifyReleaseChecksum(checksums []byte, assetName string, tarball []byte) error {
	expected, ok := findChecksumEntry(checksums, assetName)
	if !ok {
		return fmt.Errorf("checksum entry for %s not found", assetName)
	}

	sum := sha256.Sum256(tarball)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func findChecksumEntry(checksums []byte, assetName string) (string, bool) {
	for _, rawLine := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(rawLine)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "./")
		if name == assetName {
			return fields[0], true
		}
	}
	return "", false
}

func extractBinaryFromTarball(tarball []byte, assetBase string) ([]byte, os.FileMode, error) {
	gr, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, 0, fmt.Errorf("open release archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	target := path.Clean(assetBase + "/vibegram")
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read release archive: %w", err)
		}
		if hdr.FileInfo().Mode().IsRegular() && path.Clean(hdr.Name) == target {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, 0, fmt.Errorf("read extracted binary: %w", err)
			}
			mode := hdr.FileInfo().Mode().Perm()
			if mode == 0 {
				mode = 0o755
			}
			return data, mode, nil
		}
	}

	return nil, 0, fmt.Errorf("release archive did not include %s", target)
}

func backupAndInstallExecutable(executablePath string, binary []byte, mode os.FileMode) (string, error) {
	info, err := os.Stat(executablePath)
	if err != nil {
		return "", fmt.Errorf("stat current executable: %w", err)
	}

	backupPath := executablePath + ".bak-" + time.Now().Format("20060102-150405")
	if err := copyFile(executablePath, backupPath, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("backup current executable: %w", err)
	}
	if err := writeExecutableAtomically(executablePath, binary, mode); err != nil {
		return "", fmt.Errorf("install upgraded executable: %w", err)
	}
	return backupPath, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func writeExecutableAtomically(dst string, binary []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o755
	}

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		cleanup()
		return err
	}
	return nil
}

func fetchURLBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vibegram-upgrade")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
