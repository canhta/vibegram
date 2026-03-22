#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

binary_path="${tmpdir}/vibegram"
printf '#!/bin/sh\necho hello\n' > "$binary_path"
chmod 0755 "$binary_path"

archive_path="$("${repo_root}/scripts/build-release-archive" "v1.0.0" "linux" "amd64" "$binary_path" "$repo_root" "$tmpdir")"

test -f "$archive_path"
tar -tzf "$archive_path" | grep -q '^vibegram_1.0.0_linux_amd64/vibegram$'
tar -tzf "$archive_path" | grep -q '^vibegram_1.0.0_linux_amd64/README.md$'
tar -tzf "$archive_path" | grep -q '^vibegram_1.0.0_linux_amd64/LICENSE$'
tar -tzf "$archive_path" | grep -q '^vibegram_1.0.0_linux_amd64/vibegram.service$'
