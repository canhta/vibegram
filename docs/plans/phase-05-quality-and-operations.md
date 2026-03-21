# Phase 5: Quality, Release Gate, and VPS Ops

## Goal

Finish the quality bar around the system: evals, smoke tests, and production-ready VPS packaging.

## Depends on

- [Phase 3: Telegram Routing and Session State](./phase-03-telegram-and-session-state.md)
- [Phase 4: Roles, Policy, and Memory](./phase-04-roles-policy-and-memory.md)

## References

- [Testing and Evals](../testing-evals.md)
- [Runtime and Ops](../runtime-ops.md)
- [Trust Boundaries](../trust-boundaries.md)
- [Implementation Plan Overview](../implementation-plan.md)

## Deliverables

- fixture-driven eval set
- provider smoke coverage
- release-readiness gate
- `systemd` packaging

## Checklist

### Evals and release gate

- [ ] Build fixture-driven reply-safety evals
- [ ] Add provider smoke scripts for Claude
- [ ] Add provider smoke scripts for Codex
- [ ] Add authorization, redaction, and idempotent-delivery test coverage to the release gate
- [ ] Make eval success part of release readiness

### VPS packaging

- [ ] Add a production-ready `systemd` service file
- [ ] Document install, upgrade, restart, and log access
- [ ] Document sandbox profile configuration and network allowlists
- [ ] Document secret handling and service-account permissions
- [ ] Keep normal setup to a few commands
