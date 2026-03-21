# vibegram Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Track progress in the matching files under [`docs/plans/`](./plans/README.md).

**Goal:** Build the first public version of `vibegram`: a single daemon that supervises Codex and Claude Code sessions through Telegram topics with safe support-role automation.

**Architecture:** The long-term shape is still a single local daemon that owns provider runs, normalizes provider signals into stable events, stores rolling session state, and routes concise updates to one General topic plus per-session topics. The current executable slice deliberately defers Markdown retrieval and separate `ENG` / `CEO` personas in favor of one bounded support role with hardcoded rules.

**Tech Stack:** Go recommended for the daemon, Telegram Bot API, OpenAI Responses API, file-backed local state now, local SQLite later for retrieval and eval support, `systemd` for VPS deployment

---

## Plan breakdown

Tracked execution now lives in multiple phase plans:

1. [Plan Tracker](./plans/README.md)
2. [Phase 1: Foundation and Runtime](./plans/phase-01-foundation-and-runtime.md)
3. [Phase 2: Provider Ingestion and Normalization](./plans/phase-02-provider-ingestion-and-normalization.md)
4. [Phase 3: Telegram Routing and Session State](./plans/phase-03-telegram-and-session-state.md)
5. [Phase 4: Roles and Policy](./plans/phase-04-roles-policy-and-memory.md)
6. [Phase 5: Quality and Operations](./plans/phase-05-quality-and-operations.md)

Use this file as the overview and phase map.

## Release slices

The project should ship in layered executable slices, not as one giant all-or-nothing milestone:

1. executable foundation plus quiet Telegram observability
2. bounded support actions plus explicit approval packets
3. hardening, eval gates, memory expansion, and production packaging

The full plan remains intact, but delivery should stay incremental and reversible.

## File structure proposal

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
testdata/
docs/
```

## Original task map

The original task list is preserved below as a compact reference so the phase plans can trace back to it.
These checkboxes are archival reference only. Current progress is tracked in [`docs/plans/`](./plans/README.md).

## Task 1: Repo bootstrap

**Files:**
- Create: `cmd/vibegram/main.go`
- Create: `internal/config/config.go`
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`

- [ ] Define the module layout and app entrypoint.
- [ ] Add config loading for Telegram, OpenAI, provider commands, and state directory.
- [ ] Write a failing boot test that starts config parsing with sample values.
- [ ] Implement the minimal app bootstrap to make the test pass.
- [ ] Add an initial local run command.

## Task 2: Session and run state

**Files:**
- Create: `internal/state/session.go`
- Create: `internal/state/store.go`
- Create: `internal/state/store_test.go`

- [ ] Define `session_id`, `run_id`, topic IDs, status, phase, and escalation fields.
- [ ] Write tests for create, load, update, and restart-safe persistence.
- [ ] Implement file-backed or SQLite-backed state storage.
- [ ] Verify restart behavior.

## Task 3: Runner

**Files:**
- Create: `internal/runner/pty_runner.go`
- Create: `internal/runner/runner_test.go`

- [ ] Write runner tests for process launch, PTY capture, shutdown, and failure detection.
- [ ] Implement direct PTY runner.
- [ ] Expose a clean interface for provider-specific launch args.
- [ ] Keep `tmux` out of the critical path.

## Task 4: Provider adapters

**Files:**
- Create: `internal/providers/claude/adapter.go`
- Create: `internal/providers/codex/adapter.go`
- Create: `internal/providers/provider.go`
- Create: `internal/providers/providers_test.go`

- [ ] Write parser fixtures for Claude hooks/transcripts and Codex transcripts.
- [ ] Implement raw observation emission for each provider.
- [ ] Implement signal priority rules.
- [ ] Verify duplicate raw observations are still distinguishable before normalization.

## Task 5: Event normalization

**Files:**
- Create: `internal/events/normalize.go`
- Create: `internal/events/types.go`
- Create: `internal/events/dedupe.go`
- Create: `internal/events/normalize_test.go`

- [ ] Define the balanced normalized event set.
- [ ] Preserve trust provenance on normalized events.
- [ ] Write table-driven tests for all supported event mappings.
- [ ] Implement dedupe keys and replay protection.
- [ ] Verify one logical event becomes one normalized event.

## Task 6: Telegram routing and rendering

**Files:**
- Create: `internal/telegram/router.go`
- Create: `internal/telegram/render.go`
- Create: `internal/telegram/render_test.go`

- [ ] Write tests for General topic vs session topic routing.
- [ ] Write renderer tests for readable, low-noise output.
- [ ] Implement concise message formatting.
- [ ] Verify Telegram limits are respected.

## Task 7: Rolling snapshot

**Files:**
- Create: `internal/state/snapshot.go`
- Create: `internal/state/snapshot_test.go`

- [ ] Write snapshot update tests for every normalized event type.
- [ ] Implement bounded recent event storage and summary fields.
- [ ] Verify restart persistence and recovery.

## Task 8: Markdown memory and retrieval

**Files:**
- Create: `internal/retrieval/index.go`
- Create: `internal/retrieval/index_test.go`
- Create: `memory/rules/global.md`
- Create: `memory/rules/eng.md`
- Create: `memory/rules/ceo.md`

- [ ] Implement local retrieval over Markdown truth.
- [ ] Write tests for role- and trigger-aware retrieval.
- [ ] Keep SQLite FTS or equivalent simple and local.
- [ ] Verify Markdown remains the source of truth.

## Task 9: Role execution

**Files:**
- Create: `internal/roles/client.go`
- Create: `internal/roles/eng.go`
- Create: `internal/roles/ceo.go`
- Create: `internal/roles/roles_test.go`

- [ ] Define structured output contracts for `reply`, `escalate`, and `noop`.
- [ ] Implement GPT-5-family role calls via OpenAI Responses.
- [ ] Keep prompt prefixes stable for caching.
- [ ] Verify malformed or incomplete role outputs fail closed.

## Task 10: Policy engine

**Files:**
- Create: `internal/policy/policy.go`
- Create: `internal/policy/policy_test.go`

- [ ] Write tests for safe auto-reply, escalation, cooldown, and retry ceiling.
- [ ] Implement role selection and decision handling.
- [ ] Implement source-to-sink risk checks before autonomous replies.
- [ ] Verify repeated blockers escalate instead of looping.

## Task 11: Human teaching capture

**Files:**
- Create: `internal/telegram/teaching.go`
- Create: `internal/telegram/teaching_test.go`
- Create: `memory/decisions/.gitkeep`

- [ ] Define how a human override becomes a Markdown decision record.
- [ ] Write tests for decision file creation and retrieval visibility.
- [ ] Do not auto-promote rules without eval passage.

## Task 12: Evals and release gate

**Files:**
- Create: `testdata/evals/`
- Create: `internal/policy/eval_test.go`
- Create: `docs/testing-evals.md`

- [ ] Build fixture-driven reply-safety evals.
- [ ] Add provider smoke scripts for Claude and Codex.
- [ ] Make eval success part of release readiness.

## Task 13: VPS packaging

**Files:**
- Create: `packaging/vibegram.service`
- Create: `docs/runtime-ops.md`

- [ ] Add a production-ready `systemd` service file.
- [ ] Document install, upgrade, restart, and log access.
- [ ] Keep normal setup to a few commands.
