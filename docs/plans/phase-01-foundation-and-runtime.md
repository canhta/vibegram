# Phase 1: Foundation and Runtime

## Goal

Create the runnable base of the daemon: module layout, config loading, persistent session state, and a direct runner with least-privilege defaults.

## Depends on

- none

## References

- [Architecture](../architecture.md)
- [Runtime and Ops](../runtime-ops.md)
- [Go Guidance](../go-guidance.md)
- [Trust Boundaries](../trust-boundaries.md)

## Deliverables

- initial Go module and entrypoint
- config loading
- session and run persistence
- direct PTY runner
- sandbox profile model

## Checklist

### Repo bootstrap

- [ ] Define the module layout and app entrypoint
- [ ] Add config loading for Telegram, OpenAI, provider commands, and state directory
- [ ] Write a failing boot test with sample config values
- [ ] Implement the minimal app bootstrap to make the test pass
- [ ] Add an initial local run command once it exists

### Session and run state

- [ ] Define `session_id`, `run_id`, topic IDs, status, phase, and escalation fields
- [ ] Write tests for create, load, update, and restart-safe persistence
- [ ] Implement file-backed or SQLite-backed state storage
- [ ] Verify restart behavior

### Runner and sandbox

- [ ] Write runner tests for process launch, PTY capture, shutdown, and failure detection
- [ ] Implement the direct PTY runner
- [ ] Define sandbox profiles with least-privilege defaults
- [ ] Keep network disabled by default
- [ ] Expose a clean interface for provider-specific launch args
- [ ] Keep `tmux` out of the critical path
