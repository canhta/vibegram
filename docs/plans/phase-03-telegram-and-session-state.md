# Phase 3: Telegram Routing and Session State

## Goal

Wire the daemon to Telegram both ways: send filtered events to General and session topics, and receive human messages from Telegram and route them back to the right live agent session. Keep a durable rolling snapshot per app session.

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

- General topic routing (outbound)
- session topic routing (outbound)
- concise Telegram renderer
- Telegram long-polling loop (inbound)
- inbound message router: topic → session → agent PTY stdin
- General topic command handler (start / stop / status)
- Telegram user authorization (admin / operator / observer)
- rolling session snapshot store
- delivery ledger for idempotent visible sends

## Checklist

### Telegram outbound: routing and rendering

- [ ] Write tests for General topic vs session topic routing
- [ ] Write renderer tests for readable, low-noise output
- [ ] Implement concise message formatting
- [ ] Verify Telegram message limits are respected
- [ ] Implement delivery-ledger checks to prevent duplicate visible sends on restart or replay

### Telegram inbound: 2-way handler

- [ ] Implement long-polling loop for incoming Telegram updates
- [ ] Map Telegram user IDs to roles: `admin`, `operator`, `observer`
- [ ] Route incoming session-topic messages to the matching live agent session
- [ ] Inject authorized human replies into the agent process via PTY stdin
- [ ] Handle General topic commands: `start <session>`, `stop <session>`, `status`
- [ ] Reject or log messages from unauthorized users without action
- [ ] Write tests for inbound routing, authorization, and PTY injection

### Rolling snapshot

- [ ] Write snapshot update tests for every normalized event type
- [ ] Implement bounded recent event storage
- [ ] Implement summary fields for files, tests, blocker, and role activity
- [ ] Add trust-related fields: `sandbox_profile`, `pending_elevation`, `evidence_refs`
- [ ] Track session owner and last human actor for auditability
- [ ] Verify restart persistence and recovery
