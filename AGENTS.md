# AGENTS.md

Agent instructions for `vibegram`.

`vibegram` is a design-first OSS repo for a Telegram-native control room for vibecoding agents. The project is not implemented yet. Right now, the repository's source of truth is the docs under [`docs/`](/Users/canh/project/oss/vibegram/docs).

## Start Here

Before changing architecture, product behavior, or implementation plans, read:

1. [`README.md`](/Users/canh/project/oss/vibegram/README.md)
2. [`docs/decisions.md`](/Users/canh/project/oss/vibegram/docs/decisions.md)
3. [`docs/architecture.md`](/Users/canh/project/oss/vibegram/docs/architecture.md)
4. [`docs/telegram-model.md`](/Users/canh/project/oss/vibegram/docs/telegram-model.md)
5. [`docs/provider-model.md`](/Users/canh/project/oss/vibegram/docs/provider-model.md)
6. [`docs/session-context.md`](/Users/canh/project/oss/vibegram/docs/session-context.md)
7. [`docs/runtime-ops.md`](/Users/canh/project/oss/vibegram/docs/runtime-ops.md)
8. [`docs/implementation-plan.md`](/Users/canh/project/oss/vibegram/docs/implementation-plan.md)
9. [`docs/diagrams.md`](/Users/canh/project/oss/vibegram/docs/diagrams.md)

Use the deeper research docs only when the task touches those areas:

- Telegram platform details: [`docs/telegram-research.md`](/Users/canh/project/oss/vibegram/docs/telegram-research.md)
- Go implementation guidance: [`docs/go-guidance.md`](/Users/canh/project/oss/vibegram/docs/go-guidance.md)
- OpenAI role-layer guidance: [`docs/openai-guidance.md`](/Users/canh/project/oss/vibegram/docs/openai-guidance.md)
- Reference lessons from the old system: [`docs/ccgram-lessons.md`](/Users/canh/project/oss/vibegram/docs/ccgram-lessons.md)

## Product Invariants

Do not drift from these unless the user explicitly wants a design change:

- One Telegram Forum with one General topic plus per-session topics
- General topic is the control room, not just another thread
- Session topics are durable app-session rooms
- Identity is `topic -> app session_id -> run_id -> provider metadata`
- Direct process runner first
- `systemd` is the default VPS story
- `tmux` is optional later, not required for normal setup
- Claude signal priority: `hooks -> transcript -> PTY`
- Codex signal priority: `transcript -> PTY`
- Long-term memory is app-owned Markdown plus a local retrieval index
- OpenAI GPT-5-family inference is used through a role executor, not as the system of record
- `ENG` and `CEO` are on-demand roles, not always-running sidecars
- sandboxing and privilege boundaries are part of the product, not optional implementation detail
- network access is disabled by default unless explicitly elevated by policy or human approval
- Telegram should stay quiet; raw transcript streaming is not the product

## Repo Status

Current status:

- docs-first
- architecture locked
- implementation planned
- no Go code scaffolded yet

This means:

- prefer editing docs, schemas, and plans over speculative code scaffolding
- do not invent build, test, or dev commands that do not exist yet
- if you add implementation files later, keep the docs aligned in the same change

## Implementation Defaults

If the user asks to start implementation, default to the planned Go layout:

```text
cmd/vibegram/
internal/config/
internal/runner/
internal/providers/claude/
internal/providers/codex/
internal/events/
internal/state/
internal/retrieval/
internal/policy/
internal/roles/
internal/telegram/
internal/app/
```

Implementation stance:

- prefer Go standard library first
- keep one module at repo root
- use `internal/` for non-public packages
- pass `context.Context` explicitly
- use `exec.CommandContext` for provider runs
- keep adapters provider-specific and normalization provider-agnostic

## Editing Rules

When changing the design:

- update [`docs/decisions.md`](/Users/canh/project/oss/vibegram/docs/decisions.md) if a locked decision changes
- update the affected topic docs, not just one summary page
- update [`docs/diagrams.md`](/Users/canh/project/oss/vibegram/docs/diagrams.md) when flows or boundaries change
- keep schemas in [`docs/schemas/`](/Users/canh/project/oss/vibegram/docs/schemas) aligned with the written design

Do not quietly reintroduce rejected ideas:

- no terminal-mirror-first UX
- no `topic = tmux window = session` product model
- no cloud control plane in v1
- no cross-provider handoff in v1
- no self-editing memory system
- no Telegram Mini App dependency for the core workflow

## Quality Bar

Good changes in this repo are:

- explicit
- boring
- low-ambiguity
- easy to diff
- easy to test later

Prefer:

- concise docs with strong headings
- concrete examples
- small schemas
- Mermaid diagrams for flows and state

Avoid:

- vague future-speak
- duplicated requirements across many docs
- implementation detail that contradicts locked product decisions

## Validation

For doc-only changes:

- check internal consistency across README, decisions, and the affected design docs
- make sure diagrams and schemas still match the prose

For future code changes:

- add or update tests for the code you touch
- document any new run or test command here once it becomes real

## Growing This File

Keep this root file concise.

If the codebase grows, add nested `AGENTS.md` files inside `cmd/`, `internal/`, or other major subtrees instead of turning this root file into a long manual.
