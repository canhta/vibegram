# vibegram

Telegram-native control room for vibecoding agents.

`vibegram` is a local-first daemon that runs coding agents, filters their noisy output into human-readable Telegram updates, and lets support roles unblock the main agent automatically until a genuinely critical decision needs a human.

Status: design-first repo. This repository currently contains the product docs, system design, schemas, and implementation plan for the first public version.

## What vibegram is

- A single local daemon or VPS service
- A Telegram Forum workflow with one General topic plus per-session topics
- A direct process runner first, with optional `tmux` later if needed
- A system that supports both Codex and Claude Code
- A rule-driven event normalizer that turns raw agent output into clean updates
- A support layer with `ENG` and `CEO` roles that can reply directly to the main agent when safe

## What vibegram is not

- Not a terminal mirror
- Not a cloud control plane
- Not a cross-provider handoff system in v1
- Not a self-editing memory system
- Not a Telegram Mini App product

## Product stance

The main agent should keep moving without forcing the human to babysit every clarification. Telegram should stay calm. The human should see the right thing at the right time:

- General topic: control room, triage, blocked/done/critical alerts
- Session topic: important session events, auto-reply notes, escalations, and outcomes

## Core principles

1. Direct runner first. `systemd` owns uptime; `tmux` is optional.
2. Topic UX first. Telegram topics are views into state, not the state store.
3. App-owned identity. A topic binds to an app session, not a child process.
4. Markdown memory first. Rules and learned decisions live in files you can inspect and diff.
5. GPT-5 family for inference. OpenAI Responses features are accelerators, not the source of truth.
6. Safe automation. `ENG` and `CEO` reply directly only when policy says it is safe.

## Docs

- [Docs Index](./docs/README.md)
- [Vision](./docs/vision.md)
- [Decisions](./docs/decisions.md)
- [Architecture](./docs/architecture.md)
- [Telegram Model](./docs/telegram-model.md)
- [Provider Model](./docs/provider-model.md)
- [Event Model](./docs/event-model.md)
- [Session Context](./docs/session-context.md)
- [Automation Safety](./docs/automation-safety.md)
- [Runtime and Ops](./docs/runtime-ops.md)
- [Telegram Research](./docs/telegram-research.md)
- [Go Guidance](./docs/go-guidance.md)
- [OpenAI Guidance](./docs/openai-guidance.md)
- [Lessons from ccgram](./docs/ccgram-lessons.md)
- [Testing and Evals](./docs/testing-evals.md)
- [Implementation Plan](./docs/implementation-plan.md)

## Publishing note

This repo is ready for public publishing once the remote destination and license are confirmed.
