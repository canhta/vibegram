# Runtime and Ops

## Default deployment model

`vibegram` should be easiest to run like any normal Linux service:

```text
systemd
  -> vibegram daemon
     -> direct PTY child processes
```

## Why not tmux-first

`tmux` is a useful operator tool, but a poor default onboarding story.

The user should not need to understand:

- panes
- sessions
- reattach flow
- server cleanup

just to supervise agents from Telegram.

## Optional tmux adapter

`tmux` remains a future adapter for:

- manual inspection
- weird VPS PTY edge cases
- users who prefer terminal-native operational workflows

## systemd service goals

- restart on failure
- logs via journald
- startup on boot
- predictable environment loading

## Suggested runtime directories

```text
/var/lib/vibegram/      persistent app state
/var/log/vibegram/      optional file logs
/etc/vibegram/          config and environment
```

## Config surface

Recommended config:

- Telegram bot token
- Telegram forum chat ID
- OpenAI API key
- default GPT-5 family model
- provider commands or paths
- work root
- state dir
- log level
- automation mode
- sandbox profile defaults
- allowlisted network destinations

## Observability

Minimum observability:

- daemon log
- per-session state file
- per-run metadata file
- counters for auto-reply, escalation, dedupe, and failure

## Crash recovery

On daemon restart:

1. load app sessions
2. restore run metadata
3. restore offsets/checkpoints
4. resume monitoring
5. avoid replaying stale events

The system should prefer correctness over fancy self-healing.

## Sandbox requirement

The runtime must enforce least privilege by default:

- workspace-scoped file access
- network disabled by default
- explicit elevation path for higher-risk operations

Exact implementation can evolve, but the product requirement is fixed.

For the design rationale, see [Trust Boundaries](./trust-boundaries.md).

## Go implementation notes

The current recommendation for the daemon implementation is Go. For the detailed language guidance behind that recommendation, see [Go Guidance](./go-guidance.md).
