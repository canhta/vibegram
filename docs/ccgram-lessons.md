# Lessons from ccgram

This file captures what the current `ccgram` codebase teaches us about the new `vibegram` OSS.

`ccgram` is not the product we want to build, but it is a strong reference system for:

- Telegram operational reality
- provider asymmetry
- anti-spam message shaping
- fallback behavior when structured signals are incomplete

## What `ccgram` gets right

## 1. Telegram needs a presentation layer

The current bot does not succeed by forwarding raw agent output directly.
It succeeds because it has multiple layers that reshape output before it hits Telegram:

- safe send and edit helpers
- entity-based formatting fallback
- rate limiting
- queue-based ordering
- batching and merging
- special handling for tool calls and interactive prompts

That is the strongest evidence that `vibegram` should keep a dedicated normalizer and renderer instead of treating Telegram as a raw transcript sink.

## 2. Tool traffic must be handled differently from normal text

`ccgram` has explicit handling for:

- `tool_use`
- `tool_result`
- in-place edits when results arrive
- grouped batches of related tool activity

This matters because tool chatter is the main source of spam in agent sessions.
The codebase shows that a normal message stream is not enough. Tool traffic needs its own shaping rules.

Implication for `vibegram`:

- keep tool-aware normalization
- keep tool-aware dedupe
- keep tool-aware rendering
- do not let raw tool output dominate a session topic

## 3. Provider asymmetry is real

`ccgram` already proves that Claude Code and Codex expose different operational surfaces.

In practice:

- Claude has stronger lifecycle signaling through hooks plus transcript reading
- Codex relies more heavily on transcript parsing and terminal-state fallback

This reinforces the current `vibegram` adapter model:

```text
Claude: hooks -> transcript -> PTY
Codex: transcript -> PTY
```

## 4. Fallback behavior matters more than elegance

The current bot has explicit fallbacks for:

- entity formatting failures
- Telegram retry-after behavior
- missing structured signals
- transcript gaps
- terminal-only interactive states

That is a useful lesson for the new OSS:

- the happy path must be structured
- the product still needs an ugly-path story when providers or Telegram behave imperfectly

## 5. Telegram control surfaces should stay compact

`ccgram` uses:

- inline keyboards
- short callback payloads
- edits instead of repeated replacement messages
- message-thread routing

This is the right style to preserve.
The current code strongly supports the idea that Telegram works best as a compact control surface, not a deeply stateful application surface.

## What `vibegram` should not copy

## 1. Topic equals runtime identity

The biggest coupling in `ccgram` is:

```text
Telegram topic -> tmux window -> provider session
```

That was a pragmatic fit for `ccgram`, but it is the wrong default for the new OSS.

`vibegram` should keep:

```text
Telegram topic -> app session -> run -> provider metadata
```

That preserves stable user-facing identity without locking the product to one runtime implementation.

## 2. Raw transcript flow is still too close to the user

Even with the current cleanup layers, `ccgram` still spends a lot of effort repairing transcript-shaped output into Telegram-shaped output.

That is a sign that raw transcript lines should not be the main product payload in the new OSS.

`vibegram` should prefer:

- normalized events
- concise summaries
- exception-driven alerts
- optional evidence, not default transcript flow

## 3. Topic-only working rooms without a General control room

`ccgram` deliberately rejects General-topic routing.
That made sense for the old product shape, but the new OSS benefits from a real control room.

`vibegram` should keep:

- one General topic for creation, overview, blocked, failed, done, and critical alerts
- one session topic per active app session

## 4. Terminal interaction should not define the whole product

Interactive terminal rescue flows are useful, but they should be the exception.
The new OSS should optimize first for:

- filtered events
- direct automation notes
- critical escalation
- calm Telegram timelines

and only then provide deeper runtime inspection when needed.

## Recommended carry-forward patterns

Carry forward:

- Telegram-safe send and edit wrappers
- rate limiting and retry handling
- compact inline controls
- source-aware provider ingestion
- explicit tool-call shaping
- fallback paths for missing structured signals

Do not carry forward as product truth:

- `1 topic = 1 tmux window = 1 session`
- raw transcript as the primary UI payload
- no General topic
- transport-level concepts leaking into user-facing identity

## Bottom line

The current codebase is strongest as evidence for one core idea:

```text
Telegram needs a carefully shaped operational feed.
```

That supports the `vibegram` direction directly.
What we are changing is not whether shaping is needed.
What we are changing is the product boundary:

- `ccgram` shapes a terminal bridge
- `vibegram` should shape an agent supervision system
