# Telegram Model

## Why Telegram Forum

Telegram Forum topics are enough for the product we want:

- one General topic for overview
- one topic per session for detail
- inline actions
- message edits
- topic-level routing

They are not enough for a rich in-topic app. That is fine. `vibegram` is a control-room product, not a Mini App product.

For the verified platform constraints behind this design, see [Telegram Research](./telegram-research.md).
For the practical shaping lessons taken from the current reference implementation, see [Lessons from ccgram](./ccgram-lessons.md).

## Supported Telegram behavior

Realistic Telegram constraints for this design:

- topics are first-class and routeable with `message_thread_id`
- message text is capped at 4096 characters after parsing
- callback payloads are capped at 64 bytes
- bots can edit messages and use inline keyboards
- forum workflows are good for routing, not for heavy UI state

Additional platform facts that matter:

- every forum has a non-deletable General topic with `id=1`
- non-General topics are message threads, not separate chats
- Telegram Web Apps in inline buttons are private-chat only

## Topic split

### General topic

Purpose:

- create new sessions
- host the slash-only `/new` draft wizard before a session topic exists
- show active session overview
- maintain the current attention queue
- alert on blocked, failed, done, and critical events
- link or jump to the session topic

Noise budget:

- very low

Authority model:

- admin or operator humans may approve elevated actions
- observers may read and discuss, but should not implicitly authorize runtime changes
- state-changing control actions stay slash-only in General

### Session topic

Purpose:

- show important events for one session
- show automation notes
- show escalations and outcomes
- preserve concise session history

Noise budget:

- moderate but curated

Authority model:

- session topics may contain normal steering and clarifications
- teaching actions and privilege elevation should still resolve through explicit approval logic

## Topic lifecycle

```text
General topic -> /new draft
  -> choose agent
  -> choose folder
  -> type task
  -> validate or launch
  -> daemon allocates app session_id
  -> daemon creates session topic
  -> daemon launches run
  -> session topic becomes the room for that session
```

Session topics should not be deleted just because a run exits. The topic represents the app session, not the child process.

The General topic should be treated specially:

- it is always present
- it is not just "topic zero"
- it is the control room for the whole bot, not another session room
- it should behave like an operations board with edited status cards, not a pure message feed

## Message classes

### General topic messages

- `/new` draft steps and confirmations
- new session created
- session blocked
- session failed
- session done
- critical escalation required
- approval packet awaiting decision

### Session topic messages

- phase changed
- files changed summary
- tests changed summary
- important tool activity
- question
- blocked
- approval needed
- auto-reply sent
- done
- failed

## Output style rules

- no raw terminal dumps by default
- one message should tell the user what happened and what matters next
- long details should be collapsed into evidence snippets or linked artifacts later

## Actions

Planned inline actions:

- open / jump to session topic
- pause automation
- resume automation
- escalate to human
- request summary
- retry classification
- approve elevation
- deny elevation
- choose elevation scope
- choose elevation duration

## Approval packet model

Approvals should be first-class Telegram objects, not implied by freeform chat.

Each approval packet should clearly show:

- what requested the elevation
- why it is needed
- requested scope
- requested duration
- risk level
- approve / deny actions
- audit trail reference

## Team authorization

The forum model implies multi-human visibility, so `vibegram` needs a minimal human-role model:

- `admin`: can configure policy, approve elevation, manage credentials, and teach the system
- `operator`: can create sessions, steer work, and approve normal workflow choices
- `observer`: read-only visibility plus non-authoritative discussion

Telegram membership alone should not imply authority to approve elevated actions.
