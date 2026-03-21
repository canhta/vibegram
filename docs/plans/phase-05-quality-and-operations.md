# Phase 5: Smoke Tests and VPS Run

## Goal

Verify the system works end-to-end against real provider sessions and wire it up to run 24/7 in the background on a VPS with a single `systemd` unit.

## Depends on

- [Phase 3: Telegram Routing and Session State](./phase-03-telegram-and-session-state.md)
- [Phase 4: Roles and Policy](./phase-04-roles-policy-and-memory.md)

## References

- [Runtime and Ops](../runtime-ops.md)
- [Trust Boundaries](../trust-boundaries.md)

## Deliverables

- provider smoke runs (Claude + Codex against real sessions)
- one `systemd` service unit file

## Deferred to v2 (not in this phase)

- fixture-driven reply-safety eval suite
- formal release-readiness gate
- install scripts, upgrade tooling, binary distribution
- ops runbook beyond basic start/stop/logs

## Checklist

### Smoke tests

- [ ] Run a real Claude Code session through the full pipeline and verify Telegram output is correct
- [ ] Run a real Codex session through the full pipeline and verify Telegram output is correct
- [ ] Verify a blocked event triggers the support role and the reply reaches the agent
- [ ] Verify a risky request escalates to the human and does not auto-reply
- [ ] Verify no duplicate Telegram messages on daemon restart

### VPS run

- [ ] Add a `systemd` service unit file (`packaging/vibegram.service`)
- [ ] Verify the daemon starts on boot, restarts on crash, and logs to journald
- [ ] Document three commands: install, check status, read logs
