# Testing and Evals

## Testing philosophy

If `vibegram` can reply directly to the main agent, the test bar is high.

Normal unit tests are not enough. We need:

- deterministic event normalization tests
- routing tests
- role decision tests
- restart/recovery tests
- provider smoke tests
- reply-safety evals

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
   -> ENG or CEO

8. Delivery
   -> direct reply to main agent
   -> mirrored Telegram note
```

## Required test classes

### Unit tests

- provider parsers
- event dedupe
- snapshot update rules
- routing rules
- policy classification
- retrieval filters
- renderer formatting

### Integration tests

- session creation flow
- Claude signal priority behavior
- Codex signal priority behavior
- restart without duplicate replay
- auto-reply note mirrored to Telegram
- escalation path to General topic

### Eval suites

#### Policy fixture evals

Examples:

- safe clarification should auto-reply
- risky secret ambiguity should escalate
- repeated blocker should escalate
- done event should produce concise CEO summary

#### Provider smoke evals

- real Claude Code smoke
- real Codex smoke

## Release gate

Before a release candidate is accepted:

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
