# Phase 4: Roles and Policy

## Goal

Implement one unified support role that auto-replies to safe blockers and a policy engine that decides when to reply, escalate, or do nothing. Human handles only critical or risky decisions.

## Depends on

- [Phase 2: Provider Ingestion and Normalization](./phase-02-provider-ingestion-and-normalization.md)
- [Phase 3: Telegram Routing and Session State](./phase-03-telegram-and-session-state.md)

## References

- [Automation Safety](../automation-safety.md)
- [Session Context](../session-context.md)
- [OpenAI Guidance](../openai-guidance.md)
- [Trust Boundaries](../trust-boundaries.md)

## Deliverables

- one support role executor (GPT-5 structured call: `reply` / `escalate` / `noop`)
- policy engine with source-sink checks
- explicit approval packets for elevated actions

## Deferred to v2 (not in this phase)

- Markdown memory and retrieval index
- separate ENG / CEO role personas
- teaching capture (human override → decision record)
- rule promotion workflow

## Checklist

### Support role executor

- [x] Define structured output contracts for `reply`, `escalate`, and `noop`
- [x] Implement a single unified support role via GPT-5-family OpenAI Responses call
- [x] Keep prompt prefixes stable for caching
- [x] Ensure prompts separate trusted policy, trusted system state, and untrusted evidence sections
- [x] Hardcode a small initial ruleset directly in the role prompt (no retrieval index needed for v1)
- [x] Ensure role-generated messages cannot recursively trigger more role calls
- [x] Verify malformed or incomplete role outputs fail closed

### Policy engine

- [x] Write tests for safe auto-reply, escalation, and retry ceiling
- [ ] Add cooldown window handling and tests
- [x] Implement source-to-sink risk checks before any autonomous reply
- [x] Add authorization checks for `admin`, `operator`, and `observer` actions
- [ ] Implement explicit approval packets for elevated actions
- [x] Verify repeated blockers escalate instead of looping
- [ ] Redact secrets before Telegram output
