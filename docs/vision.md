# Vision

## One-line product definition

`vibegram` is a Telegram-native control room for autonomous coding sessions.

## Problem

Raw agent output is noisy, unreadable, and badly shaped for Telegram. Humans are pulled into the loop too often for low-value clarifications, but when they step away entirely, the agent stalls on simple unblockers.

The current market failure is not "we need more agents." It is:

- too much raw output
- not enough signal
- weak handoff between machine decisions and human oversight
- poor remote supervision

## Desired user experience

The user starts or monitors work from Telegram.

- The General topic shows overall activity and only important alerts.
- Each session topic shows a concise timeline for that session.
- When the main agent asks something safe and local, `ENG` or `CEO` can answer directly.
- When the question is risky, repeated, ambiguous, or strategic, the human is pulled in.

## Product wedge

The first wedge is not "AI work operating system."

The first wedge is:

> Make Telegram a calm, useful supervision layer for long-running coding agents.

## Personas

### Solo builder

- runs agents locally or on a VPS
- wants to step away without losing control
- wants Telegram as a lightweight remote console, not a wall of logs

### Technical lead

- wants to monitor multiple sessions
- wants strategic summaries and escalation on critical items
- wants automation to handle low-value back-and-forth

## Non-goals

- full multi-user enterprise permissions
- cloud-hosted remote runner platform
- persistent semantic memory graph
- cross-provider conversation portability
- full raw transcript replay in Telegram

## Success criteria for v1

- A user can run the system on a VPS with one service unit.
- Telegram General topic stays low-noise.
- Session topics are readable and useful.
- Codex and Claude Code both work under the same app-owned session model.
- `ENG` and `CEO` can safely auto-reply on a bounded set of unblockers.
- Human-taught decisions are captured into Markdown and reused on future sessions.
