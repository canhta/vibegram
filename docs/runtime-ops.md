# Runtime and Ops

## Deployment default

The default production story is:

```text
systemd
  -> vibegram daemon
     -> direct PTY child processes
```

`tmux` is optional later, not part of the normal setup.

## Runtime directories

```text
/etc/vibegram/          config and secrets
/var/lib/vibegram/      state and runtime data
```

The systemd unit should pin:

- `VIBEGRAM_WORK_ROOT`
- `VIBEGRAM_STATE_DIR`
- `HOME` for the chosen service account
- `EnvironmentFile=/etc/vibegram/env`

## Install flow

Production default:

```bash
sudo vibegram install
```

That command should:

- detect the real operator account when possible
- detect `codex` and `claude` paths
- write `/etc/vibegram/env`
- make that env file readable by the service account
- install `/etc/systemd/system/vibegram.service`
- reload systemd
- enable and start the service
- print final service status

After the first install, the standard in-place Ubuntu upgrade flow is:

```bash
sudo vibegram upgrade
```

That command should:

- resolve the latest GitHub release unless `--version` is set
- download `SHA256SUMS` and the matching release tarball for the current platform
- verify the tarball checksum before changing the installed binary
- replace the current `vibegram` executable while leaving `/etc/vibegram/env` and the systemd unit alone
- restart the `vibegram` service
- print final service status

Pinned release upgrades should also work:

```bash
sudo vibegram upgrade --version v0.1.1
```

Advanced split flow still exists:

```bash
sudo vibegram init
sudo vibegram service install
sudo vibegram service start
sudo vibegram service status
sudo vibegram service logs
```

## Config surface

Expected config:

- Telegram bot token
- Telegram forum chat ID
- admin/operator Telegram IDs
- `VIBEGRAM_PROVIDER_CODEX_CMD`
- `VIBEGRAM_PROVIDER_CLAUDE_CMD`
- optional OpenAI-compatible API settings
  - `VIBEGRAM_OPENAI_MODEL` defaults to `gpt-5-mini` for the support layer
  - `VIBEGRAM_OPENAI_STRONG_MODEL` defaults to `gpt-5` and is used only for higher-ambiguity support work
- work root
- state dir
- log level

Support-cost defaults should stay boring:

- obvious General-topic help should be deterministic when possible
- repeated blocker/question summaries should not trigger fresh support calls
- risky support situations should escalate directly
- auto-replies should stay budgeted so one stuck session cannot cause unbounded extra provider resumes

If Codex or the support role is pointed at an OpenAI-compatible reverse proxy, that proxy must allow Responses API request bodies larger than the nginx default.
For nginx, set `client_max_body_size` high enough for resumed threads.

## Service-account stance

The production default should prefer the real operator account when that account already owns provider auth and binaries.
That keeps first install simple.

If a user wants a tighter setup later, `service install --user ...` remains the escape hatch.

## Observability

Minimum operability:

- `systemctl status vibegram`
- `journalctl -u vibegram`
- persisted session/run/snapshot state
- replay-safe Telegram offset handling

## Recovery rules

On restart the daemon should:

1. load sessions
2. load runs and snapshots
3. restore update offsets
4. avoid replaying stale Telegram updates
5. resume cleanly without duplicate visible delivery
6. treat a provider turn failure as a session failure, not a daemon-fatal error

## Safety

Defaults should stay boring:

- workspace-scoped access
- no privilege widening from provider text alone
- no secret reflection into Telegram
- no terminal-mirror UX as the primary product

## Release and testing

Release readiness should include:

- `go test ./...`
- provider smoke coverage
- release artifacts for Linux and macOS
- a working Ubuntu systemd path
