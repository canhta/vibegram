# Phase 4: Roles, Policy, and Memory

## Goal

Implement bounded support actions, GPT-5-family profile execution, source-to-sink policy decisions, and the first safe memory hooks.

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

- support-action executor
- policy engine with source-sink checks
- explicit approval handling
- optional first memory hooks

## Checklist

### Markdown memory and retrieval

- [ ] Decide the thinnest v1 memory path that is actually needed
- [ ] If memory ships in v1, keep it to explicit saved decisions and lightweight retrieval
- [ ] Defer richer promotion workflows until the supervision loop proves itself

### Role execution

- [ ] Define structured output contracts for `reply`, `escalate`, and `noop`
- [ ] Implement GPT-5-family role calls via OpenAI Responses
- [ ] Keep prompt prefixes stable for caching
- [ ] Ensure prompts separate trusted policy, trusted system state, and untrusted evidence
- [ ] Ensure role-generated messages cannot recursively trigger more role calls
- [ ] Verify malformed or incomplete role outputs fail closed
- [ ] Keep persona naming internal and expose only user-facing support actions

### Policy engine and teaching capture

- [ ] Write tests for safe auto-reply, escalation, cooldown, and retry ceiling
- [ ] Implement role selection and decision handling
- [ ] Implement source-to-sink risk checks before autonomous replies
- [ ] Add authorization checks for operator, admin, and observer actions
- [ ] Implement explicit approval packets for elevated actions
- [ ] Verify repeated blockers escalate instead of looping
- [ ] Define how a human override becomes a Markdown decision record
- [ ] Write tests for decision file creation and retrieval visibility
- [ ] Redact secrets before Telegram output or Markdown memory capture
- [ ] Keep rule promotion gated behind eval passage
