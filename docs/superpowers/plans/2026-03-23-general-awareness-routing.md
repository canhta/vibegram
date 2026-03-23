# General Awareness Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `General` an awareness-only board that receives only major lifecycle and escalation events, including blocker resolution, while all follow-up stays in the session topic.

**Architecture:** Extend the normalized event set with an explicit blocker-resolved event, route only awareness-grade events to `General`, and keep routine questions and support replies in the session topic. Cover the behavior with focused tests at the event, router, renderer, and runtime levels.

**Tech Stack:** Go, Go standard library testing, existing `internal/events`, `internal/telegram`, and `internal/app` packages

---

### Task 1: Lock the awareness event contract in tests

**Files:**
- Modify: `internal/telegram/router_test.go`
- Modify: `internal/telegram/render_test.go`
- Modify: `internal/events/normalize_test.go`

- [ ] **Step 1: Write failing router tests for awareness-only delivery**
- [ ] **Step 2: Write failing normalization test for `blocker_resolved`**
- [ ] **Step 3: Write failing render test for blocker-resolved copy**
- [ ] **Step 4: Run targeted tests and confirm they fail for the expected missing behavior**

### Task 2: Implement the new event and routing behavior

**Files:**
- Modify: `internal/events/types.go`
- Modify: `internal/events/normalize.go`
- Modify: `internal/telegram/router.go`
- Modify: `internal/telegram/render.go`
- Modify: `internal/providers/codex/adapter.go`
- Modify: `internal/providers/codex/adapter_test.go`

- [ ] **Step 1: Add `EventTypeBlockerResolved` to the normalized event set**
- [ ] **Step 2: Normalize raw `blocker_resolved` observations and assign severity**
- [ ] **Step 3: Route `blocker_resolved` to both `General` and the session topic**
- [ ] **Step 4: Render blocker-resolved events with concise awareness wording**
- [ ] **Step 5: Teach the Codex adapter to classify unblock-resolution messages**
- [ ] **Step 6: Run targeted package tests and confirm they pass**

### Task 3: Verify runtime delivery in both launch and resume flows

**Files:**
- Modify: `internal/app/runtime_general_launch_test.go`
- Modify: `internal/app/runtime_test.go`

- [ ] **Step 1: Write failing runtime test for general-topic awareness delivery of blocker resolution**
- [ ] **Step 2: Write failing runtime test that routine questions stay out of `General`**
- [ ] **Step 3: Implement any minimal runtime adjustments required by the tests**
- [ ] **Step 4: Run targeted runtime tests and confirm they pass**

### Task 4: Full verification and branch handoff

**Files:**
- Modify: `README.md`
- Modify: `docs/decisions.md`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Run `go test ./...`**
- [ ] **Step 2: Review `git diff --stat` and `git status --short`**
- [ ] **Step 3: Commit the implementation and docs together**
- [ ] **Step 4: Create a PR if GitHub CLI/auth is available; otherwise provide the exact PR command and summary**
