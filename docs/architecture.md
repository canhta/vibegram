# Architecture

## System overview

`vibegram` is a single daemon that ingests provider signals, normalizes them into stable events, updates a rolling session snapshot, and routes readable output into Telegram topics.

## Top-level diagram

```text
                    +-----------------------------------+
                    | Telegram Forum                    |
                    |-----------------------------------|
                    | General topic                     |
                    | - create session                  |
                    | - blocked/done/critical alerts    |
                    | - jump to session topic           |
                    |                                   |
                    | Session topics                    |
                    | - important events                |
                    | - auto-reply notes                |
                    | - escalation notes                |
                    +----------------+------------------+
                                     ^
                                     |
                    +----------------+------------------+
                    | vibegram daemon                  |
                    |-----------------------------------|
                    | Runner                            |
                    | Provider adapters                 |
                    | Event normalizer                  |
                    | Rolling snapshot store            |
                    | Delivery ledger                   |
                    | Retrieval index                   |
                    | Policy engine                     |
                    | Trust boundary checks             |
                    | support-profile executor          |
                    +-----------+--------------+--------+
                                |              |
                    +-----------+              +-----------+
                    |                                      |
                    v                                      v
             Claude Code                               Codex
       hooks -> transcript -> PTY              transcript -> PTY
```

For the maintained visual set used across the repo, see [Diagrams](./diagrams.md).

## Major components

### 1. Runner

Responsibilities:

- launch agent processes

- capture PTY output
- track process lifecycle
- optionally support `tmux` later as an adapter

Non-responsibilities:

- no Telegram formatting
- no policy decisions
- no memory mutation logic

### 2. Provider adapters

Responsibilities:

- understand provider-specific transcript formats
- understand provider-specific lifecycle signals
- emit provider-native raw observations into the normalizer

Non-responsibilities:

- no Telegram routing
- no business-level event naming outside the normalized contract

### 3. Event normalizer

Responsibilities:

- dedupe repeated provider signals
- map raw provider observations to stable event types
- generate compact event payloads

This is the heart of the product.

### Trust boundary requirement

The architecture must preserve a hard split between:

- trusted policy
- trusted system state
- untrusted evidence

and must evaluate source-to-sink risk before any autonomous reply or privilege escalation.

For the full model, see [Trust Boundaries](./trust-boundaries.md).

### 4. Snapshot store

Responsibilities:

- keep current session state
- store recent event window
- track last blocker, last files summary, last tests summary, escalation state

This is intentionally a rolling state store, not long-term semantic memory.

### 5. Retrieval index

Responsibilities:

- index local Markdown rules and decisions
- return relevant rules/examples quickly
- keep Markdown as the source of truth

### 6. Delivery ledger

Responsibilities:

- enforce idempotent Telegram delivery
- track which normalized events have already produced visible messages
- prevent duplicate alerts on restart or multi-source replay

The product promise is low-noise supervision.
Without a delivery ledger, replay-safe offsets alone are not enough.

### 7. Policy engine

Responsibilities:

- decide whether to render, auto-reply, escalate, or ignore
- choose the right internal support profile
- enforce retry ceilings and escalation rules
- enforce authorization and elevation rules

### 8. Telegram renderer

Responsibilities:

- General topic message rendering
- session topic message rendering
- compact, human-readable output

## Data flow

```text
provider signal
   -> adapter
   -> normalized event
   -> dedupe
   -> snapshot update
   -> delivery ledger check
   -> routing decision
      -> General topic
      -> session topic
      -> role execution
      -> no-op
```

## Storage model

Recommended local storage layout:

```text
state/
  sessions/
    <session_id>.json
  runs/
    <run_id>.json
  offsets/
    claude-<run_id>.json
    codex-<run_id>.json
memory/
  rules/
    global.md
    eng.md
    ceo.md
  decisions/
    YYYY-MM/
      <slug>.md
  promotions/
    <slug>.md
index/
  retrieval.db
logs/
  daemon.log
artifacts/
  evidence/
    <event_id>.json
```

## Failure philosophy

- visible failure is better than silent failure
- dedupe aggressively
- make visible delivery idempotent
- retries are bounded
- automation stops and escalates when uncertainty rises
- the daemon owns orchestration, not recovery magic

## Reference system note

The current `ccgram` codebase is a useful reference for message shaping, provider asymmetry, and Telegram-safe fallback behavior.
For the distilled lessons we want to preserve and the couplings we explicitly want to avoid, see [Lessons from ccgram](./ccgram-lessons.md).
