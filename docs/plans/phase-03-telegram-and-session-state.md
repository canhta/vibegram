# Phase 3: Telegram Routing and Session State

## Goal

Render the normalized event stream into a calm Telegram workflow and keep a durable rolling snapshot per app session.

## Depends on

- [Phase 1: Foundation and Runtime](./phase-01-foundation-and-runtime.md)
- [Phase 2: Provider Ingestion and Normalization](./phase-02-provider-ingestion-and-normalization.md)

## References

- [Telegram Model](../telegram-model.md)
- [Session Context](../session-context.md)
- [Diagrams](../diagrams.md)
- [Telegram Research](../telegram-research.md)
- [Lessons from ccgram](../ccgram-lessons.md)

## Deliverables

- General topic routing
- session topic routing
- concise Telegram renderer
- rolling session snapshot store
- delivery ledger for idempotent visible sends

## Checklist

### Telegram routing and rendering

- [ ] Write tests for General topic vs session topic routing
- [ ] Write renderer tests for readable, low-noise output
- [ ] Implement concise message formatting
- [ ] Verify Telegram message and callback limits are respected
- [ ] Implement delivery-ledger checks to prevent duplicate visible sends

### Rolling snapshot

- [ ] Write snapshot update tests for every normalized event type
- [ ] Implement bounded recent event storage
- [ ] Implement summary fields for files, tests, blocker, and role activity
- [ ] Add trust-related fields such as `sandbox_profile`, `pending_elevation`, and `evidence_refs`
- [ ] Track session owner and last human actor for auditability
- [ ] Verify restart persistence and recovery
