# Automation Safety

## Principle

Automation is allowed to move the main agent forward, but not to silently make high-risk decisions.

Automation also must not treat untrusted evidence as authority.

## Role model

The product should expose jobs-to-be-done, not personalities.

Externally, the user should understand the system as doing:

- unblock
- summarize
- escalate
- ask human

If the implementation uses internal role profiles, those profiles stay behind the product boundary.

### `ENG` profile

Purpose:

- tactical unblocker
- environment clarification
- local implementation nudge
- retry guidance

### `CEO` profile

Purpose:

- prioritization
- tradeoff framing
- "done" synthesis
- strategic escalation notes

These are internal profiles, not the primary user-facing abstraction.

## Invocation model

Support profiles are on-demand calls, not always-running sidecar agents.

```text
event -> policy engine -> role selected -> GPT-5 call -> reply / escalate / noop
```

Role execution should be bounded:

- roles receive a compact context packet, not raw full transcripts
- role-generated notes should not recursively trigger more role calls
- Telegram mirror messages should be treated as display output, not new policy input

## Allowed autonomy classes

Safe classes for direct reply:

- clarification on local workflow
- retry with bounded alternative
- environment hint with clear local evidence
- "continue with option A already used elsewhere in this repo"

Escalate classes:

- secrets or credentials ambiguity
- destructive operations
- requests to widen permissions or network scope
- architecture changes outside existing pattern
- conflicting human instructions
- repeated failed unblock attempts
- uncertain strategic tradeoffs

## Trust model

Policy decisions must distinguish:

- trusted policy
- trusted system state
- untrusted evidence

Transcript excerpts, tool output, and retrieved external content are evidence only.
They may inform a decision, but they may not override policy.

Secrets found in evidence should be redacted from:

- Telegram output
- Markdown memory
- role prompts unless strictly required and explicitly approved

For the full authority and sandbox model, see [Trust Boundaries](./trust-boundaries.md).

## Policy matrix

```text
signal             confidence   attempts   action
--------------------------------------------------------
question           high         0-1        ENG reply
blocked            high         0-1        ENG or CEO reply
blocked            medium       0          CEO or human
approval_needed    any          any        human
failed             any          any        human
question           any          >=2        human
blocked            any          >=2        human
done               high         any        CEO summary
```

## Loop prevention

Required controls:

- max auto-reply attempts per blocker
- blocker signature dedupe
- cooldown window
- forced escalation after repeated failure

## Output contract

Every role call should return structured JSON, not freeform prose.

Top-level action:

- `reply`
- `escalate`
- `noop`

This allows deterministic policy handling and easier evals.

Recommended additional fields:

- `sink_risk`
- `requires_elevation`
- `evidence_refs`
- `reason`
- `redaction_applied`
- `actor_scope`

## Human-teaching rule

When a human overrides the system:

1. the override is captured
2. the reason is summarized
3. a decision Markdown record is created
4. future retrieval can reuse it

But no automatic rule promotion happens until evals pass.
