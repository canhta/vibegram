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
- General topic command handler (`status` first, then `start` / `stop`)
- Telegram user authorization (admin / operator / observer)
- rolling session snapshot store
- delivery ledger for idempotent visible sends

## Checklist

### Telegram outbound: routing and rendering

- [x] Write tests for General topic vs session topic routing
- [x] Write renderer tests for readable, low-noise output
- [x] Implement concise message formatting
- [x] Verify Telegram message limits are respected
- [x] Implement delivery-ledger checks to prevent duplicate visible sends on restart or replay

### Telegram inbound: 2-way handler

- [ ] Implement long-polling loop for incoming Telegram updates
- [x] Map Telegram user IDs to roles: `admin`, `operator`, `observer`
- [x] Route incoming session-topic messages to the matching live agent session
- [x] Inject authorized human replies into the agent process via PTY stdin
- [x] Handle General topic command: `status`
- [ ] Implement General topic commands: `start <session>`, `stop <session>`
- [x] Reject or log messages from unauthorized users without action
- [x] Write tests for inbound routing, authorization, and PTY injection

### Rolling snapshot

- [x] Write snapshot update tests for every normalized event type
- [x] Implement bounded recent event storage
- [x] Implement summary fields for files, tests, and blocker activity
- [ ] Track last role activity in rolling snapshot state
- [ ] Add `sandbox_profile` to session or snapshot state
- [ ] Persist trust-related fields in the rolling snapshot; `pending_elevation` and `evidence_refs` currently live on the session record only
- [x] Track session owner and last human actor for auditability
- [x] Verify restart persistence and recovery
