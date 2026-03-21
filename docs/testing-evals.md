# Testing and Evals

## Testing philosophy

If `vibegram` can reply directly to the main agent, the test bar is high.

This document describes the target release-quality test bar. The current executable slice only ships unit tests and package-level coverage; retrieval, eval fixtures, smoke scripts, and the formal release gate are still pending work.

Normal unit tests are not enough. We need:

- deterministic event normalization tests
- routing tests
- role decision tests
- restart/recovery tests
- provider smoke tests
- reply-safety evals
- authorization and elevation tests
- idempotent delivery tests
- redaction tests

## Core test diagram

```text
1. General topic create session
   -> app session allocated
   -> session topic created
   -> run launched

2. Provider ingestion
   Claude: hooks -> transcript -> PTY
   Codex: transcript -> PTY

3. Event normalization
   -> dedupe
   -> event emitted once

4. Snapshot update
   -> recent state updated

5. Routing
   -> General topic or session topic or both

6. Policy
   -> reply / escalate / noop

7. Role call
   -> single support role

8. Delivery
   -> direct reply to main agent
   -> mirrored Telegram note

9. Authorization and elevation
   -> operator or admin approval path
   -> no privilege widening without explicit authority
```

## Required test classes

### Unit tests

- provider parsers
- event dedupe
- snapshot update rules
- routing rules
- policy classification
- support-role decision handling
- renderer formatting
- delivery-ledger behavior
- redaction behavior

### Integration tests

- session creation flow
- Claude signal priority behavior
- Codex signal priority behavior
- restart without duplicate replay
- auto-reply note mirrored to Telegram
- escalation path to General topic
- no duplicate Telegram message after restart or replay
- unauthorized human cannot approve elevated action

### Eval suites

#### Policy fixture evals

Examples:

- safe clarification should auto-reply
- risky secret ambiguity should escalate
- repeated blocker should escalate
- done event should stay quiet until summary behavior is explicitly added
- privilege widening request should escalate
- evidence containing secrets should be redacted

#### Provider smoke evals

- real Claude Code smoke
- real Codex smoke

## Release gate

Once the eval slice lands, a release candidate should require:

1. unit and integration tests pass
2. reply-safety eval gate passes
3. provider smoke runs pass
4. General topic remains low-noise in manual review

## Critical failure modes

### Duplicate event spam

Risk:

- same logical event appears from hook and transcript

Required defense:

- dedupe key
- replay-safe offsets
- delivery ledger
- integration test

### Auto-reply loop

Risk:

- agent asks again
- support role keeps retrying

Required defense:

- blocker signature
- attempt ceiling
- forced escalation

### Silent stale state after restart

Risk:

- daemon replays or misses events

Required defense:

- persisted offsets
- startup reconciliation
- restart integration test

### Bad policy classification

Risk:

- unsafe auto-reply where a human should be asked

Required defense:

- eval fixtures
- confidence thresholds
- escalation fallback

### Silent privilege widening

Risk:

- an autonomous role or unauthorized human broadens access without a clear approval record

Required defense:

- authorization checks
- explicit elevation event
- audit trail
