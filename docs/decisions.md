# Locked Decisions

This file records the major product and engineering decisions already made.

## Product

1. Product type: brand-new OSS, not an evolution of the current `ccgram` codebase
2. Surface: Telegram Forum with one General topic plus per-session topics
3. Goal: quiet supervision, not terminal mirroring
4. Human role: only critical items require a human
5. User-facing support actions are `unblock`, `summarize`, `escalate`, and `ask human`
6. Any `ENG` / `CEO` style role naming is internal implementation detail

## Runtime

1. Deployment shape: single local daemon/binary
2. VPS story: `systemd` is the default supervisor
3. Runner: direct PTY subprocess runner first
4. `tmux`: optional fallback later, not part of normal setup

## State and identity

1. Session identity is app-owned
2. A session has a stable `session_id`
3. Each launch or resume attempt gets a `run_id`
4. Provider session IDs and PIDs are attached metadata, not the primary identity

## Provider support

1. Claude Code is supported
2. Codex is supported
3. Claude signal priority: hooks, then transcript, then PTY fallback
4. Codex signal priority: transcript, then PTY fallback

## Telegram model

1. General topic is the control room
2. Session topics are the working rooms
3. General topic is awareness-first; it receives concise summary events, not routine session follow-up
4. General topic receives new session, needs human or unblock requested, blocker resolved, done, failed, and critical escalation events
5. Session topics receive important session events, automation notes, routine support exchange, and all follow-up actions for that session
6. Session creation starts with a slash-only `/new` draft in General; once provider, folder, and task are set, the next task message creates the session topic and launches the run directly

## Context and memory

1. Context model: rolling session snapshot
2. Long-term learning: Markdown-first, app-owned
3. Retrieval: local index, not embeddings-first
4. Self-editing prompts: rejected for v1

## LLM and safety

1. Model family: OpenAI GPT-5 family
2. API style: Responses API plus structured outputs
3. OpenAI conversation state: optimization only, not the source of truth
4. Full reply-safety eval gate is required
7. `ENG` and `CEO` cannot grant new privileges on their own
8. Policy decisions must preserve trusted-vs-untrusted provenance

## Deferred

1. Optional `tmux` runner adapter after the direct runner is stable
