# Telegram Design System

`vibegram` is a Telegram-native control room for coding agents.
Its UX should feel like calm delegation with BotFather discipline, not a chatty assistant and not a raw ops console.

## Product Stance

- calm delegation
- quiet by default
- attention-first when a human is needed
- support behavior is visible and auditable
- progress is filtered into signal, not transcript noise

## BotFather Lens

Use BotFather as the interaction reference for Telegram-native quality:

- command-first
- explicit state changes
- short replies
- buttons for safe next actions
- progressive disclosure instead of long explanations
- clear confirmation for destructive actions
- no filler copy

`vibegram` should feel more capable than BotFather, but just as legible.

## Surface Model

There are two distinct Telegram surfaces:

- `General`: the control room
- session topics: the working rooms

### General

`General` is attention-first.
When an operator opens it after time away, the product should answer:

1. what needs me now?
2. what can I do next?
3. what important things already moved forward?

`General` should be composed of:

```text
PERSISTENT CONTROL CARD
  -> needs you now
  -> active session counts
  -> recent support decisions worth awareness

AWARENESS STREAM
  -> session started
  -> support replied
  -> blocker resolved
  -> done
  -> failed
```

`General` is not:

- a transcript mirror
- a session-by-session backlog dump
- a place for routine support back-and-forth

### Session Topics

Each session topic is a durable working room.
It should be understandable without scrolling far back.

Each session topic should contain:

```text
PERSISTENT SESSION HEADER CARD
  -> agent
  -> folder
  -> goal
  -> current state
  -> support state
  -> latest support decision
  -> whether a human is needed now

ACTIVE THREAD
  -> the latest blocker, question, reply, or outcome

EVIDENCE
  -> files
  -> tests
  -> key tool activity
```

## Hierarchy Rules

### General hierarchy

1. attention requiring human action
2. control actions
3. awareness

Attention items outrank everything else.

### Session hierarchy

1. what this session is for
2. what state it is in now
3. what happens next
4. the proof trail of meaningful work

## Support Visibility

Support is part of the product, not an invisible side effect.

Every session has a visible support state:

- `Support: idle`
- `Support: replied`
- `Support: ask human`
- `Support: escalated`

The latest support decision should be shown in the session header card.

`General` should receive support awareness only for meaningful decisions.
Those awareness items should use action-plus-rationale summaries, not raw quoted transcript.

Support should also be cost-aware:

- do not spend a model call on obvious slash-command guidance
- do not spend repeated model calls on the same blocker/question state
- prefer direct escalation over model consultation for clearly risky situations
- keep automatic support replies budgeted so one session cannot monopolize inference spend

Example:

```text
Support replied in Desktop codex 1651: chose Go stdlib tests.
```

## Message Generation Model

User-visible Telegram copy should not be fully hardcoded.
It should also not be free-form LLM improvisation.

Every Telegram-visible message should be produced from a typed UI message spec.

The spec defines:

- surface: `General` or session topic
- intent: `attention`, `command`, `awareness`, `recovery`, or `support`
- urgency: `low`, `medium`, or `high`
- required facts
- optional context
- next-action policy
- maximum length
- whether freeform phrasing is allowed

### System-owned fields

The system owns:

- whether a message is shown
- where it is shown
- urgency
- support decision type
- buttons
- escalation level
- state transitions

### LLM-owned fields

The LLM may shape:

- wording
- compression
- friendliness
- explanation tone

The LLM may not decide routing, urgency, or action availability.

## Tone

Default tone:

- calm
- concise
- operator-friendly
- slightly formal
- confident without hype

State-specific tone:

- success: clear, not celebratory
- failure: direct, not alarming
- blocker: specific, not dramatic
- support: accountable, not verbose

Interaction style:

- prefer directives over chatter
- prefer short labeled facts over paragraphs
- prefer buttons over prose when the next move is safe and obvious
- do not narrate internal reasoning unless it helps the operator decide

## State Design Rules

Every important Telegram state must include:

1. what happened
2. what `vibegram` is doing or not doing
3. one obvious next action

If the next action is obvious and safe, prefer a button.
If the next action is risky or ambiguous, use concise text guidance.

### Action-first state table

```text
FEATURE            | WAITING                         | EMPTY                              | ERROR / EXPIRED                           | SUCCESS
-------------------|----------------------------------|------------------------------------|-------------------------------------------|-----------------------------------------
/status            | Checking active sessions...     | No active sessions. Start one?     | Status is temporarily unavailable.        | Operator card
/new               | Preparing agent choices...      | No recent folders yet. Start from Home. | That draft expired. Resume draft?   | Draft summary and launch handoff
folder browse      | Loading folders...              | Nothing useful here yet. Go up?    | That folder is gone now. Pick another.    | Current path and Choose Here
/cleanup           | Looking for session topics...   | No session topics to clean up.     | That topic is already gone.               | Deleted N topics
session start      | Creating topic and launching... | n/a                                | Launch failed before the session started. | Session header and first progress
session topic      | Agent is working...             | No new visible update yet.         | This topic is no longer linked.           | Progress, blocker, or done
support            | Support is reviewing...         | Support idle.                      | Support decision unavailable.             | Replied, ask human, or escalated
```

### Draft recovery

`/new` should be resume-friendly.

- preserve provider choice when safe
- preserve folder choice when safe
- if the folder is gone, return to folder selection instead of hard reset
- if callbacks are stale, show the saved draft summary and offer:
  - `Resume draft`
  - `Start over`
  - `Cancel`

## Telegram Mobile And Accessibility

Telegram is the UI, so readability rules are part of the design system.

### Mobile rules

- put the most important fact in the first line
- keep operational messages scannable in at most 3 visual chunks:
  - state
  - key fact
  - next action
- avoid dense paragraph blocks for operational moments
- prefer 1 to 2 buttons per row
- prefer at most 5 actionable buttons in one message when possible
- truncate long paths and summaries intentionally, not mid-meaning

### Accessibility rules

- message order must make sense when read linearly
- do not rely on emoji-only or punctuation-only state markers
- support state and urgency must be expressed in words
- buttons must use verbs, not vague labels
- topic names and status text should stay understandable without recent context

## Topic Titles

Topic titles should stay short, operator-friendly, and skimmable.

Pattern:

```text
{folder} {provider} {short_code}
```

The goal is fast recognition, not decorative naming.

## Canonical Message Examples

These are product examples, not hardcoded strings.
Generated copy should stay recognizably within this shape.

### General control card

```text
Control room
Needs you now: 2
Active sessions: 4
Support decisions: 1 recent

Waiting on you
- Desktop codex 1651 -> ask human: confirm release scope
- api claude 4207 -> failed: resume request rejected
```

### Session header card

```text
Desktop codex 1651
Agent: Codex
Folder: ~/Desktop
Goal: add upgrade command
State: blocked
Support: ask human
Decision: confirm release version before continuing
You: needed now
```

### Support reply awareness in General

```text
Support replied in Desktop codex 1651: chose Go stdlib tests.
```

### Support escalation awareness in General

```text
Support escalated in api claude 4207: approval needed for production access.
```

### Recovery state

```text
That folder is no longer available.
Your draft is still here.
Pick another folder to continue.
```

### Empty state

```text
No active sessions right now.
Start a new one when you're ready.
```

## Not In Scope

The Telegram UX should not drift into these patterns:

- transcript-first rendering
- free-form LLM control over routing or urgency
- General as a purely chronological feed
- invisible support intervention
- decorative, emoji-heavy system messaging
- session topics that require scrolling to recover context
