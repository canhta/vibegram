# Phase 4: Roles, Policy, and Memory

## Goal

Implement local Markdown-backed memory, GPT-5-family role execution, source-to-sink policy decisions, and human-teaching capture.

## Depends on

- [Phase 2: Provider Ingestion and Normalization](./phase-02-provider-ingestion-and-normalization.md)
- [Phase 3: Telegram Routing and Session State](./phase-03-telegram-and-session-state.md)

## References

- [Automation Safety](../automation-safety.md)
- [Session Context](../session-context.md)
- [OpenAI Guidance](../openai-guidance.md)
- [Trust Boundaries](../trust-boundaries.md)
- [Testing and Evals](../testing-evals.md)

## Deliverables

- local retrieval index
- role executor for `ENG` and `CEO`
- policy engine with source-sink checks
- human override capture into Markdown decisions

## Checklist

### Markdown memory and retrieval

- [ ] Implement local retrieval over Markdown truth
- [ ] Write tests for role-aware and trigger-aware retrieval
- [ ] Keep SQLite FTS or equivalent simple and local
- [ ] Verify Markdown remains the source of truth

### Role execution

- [ ] Define structured output contracts for `reply`, `escalate`, and `noop`
- [ ] Implement GPT-5-family role calls via OpenAI Responses
- [ ] Keep prompt prefixes stable for caching
- [ ] Ensure prompts separate trusted policy, trusted system state, and untrusted evidence
- [ ] Verify malformed or incomplete role outputs fail closed

### Policy engine and teaching capture

- [ ] Write tests for safe auto-reply, escalation, cooldown, and retry ceiling
- [ ] Implement role selection and decision handling
- [ ] Implement source-to-sink risk checks before autonomous replies
- [ ] Verify repeated blockers escalate instead of looping
- [ ] Define how a human override becomes a Markdown decision record
- [ ] Write tests for decision file creation and retrieval visibility
- [ ] Keep rule promotion gated behind eval passage
