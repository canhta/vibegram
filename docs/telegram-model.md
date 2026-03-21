# Telegram Model

## Why Telegram Forum

Telegram Forum topics are enough for the product we want:

- one General topic for overview
- one topic per session for detail
- inline actions
- message edits
- topic-level routing

They are not enough for a rich in-topic app. That is fine. `vibegram` is a control-room product, not a Mini App product.

## Supported Telegram behavior

Realistic Telegram constraints for this design:

- topics are first-class and routeable with `message_thread_id`
- message text is capped at 4096 characters after parsing
- callback payloads are capped at 64 bytes
- bots can edit messages and use inline keyboards
- forum workflows are good for routing, not for heavy UI state

## Topic split

### General topic

Purpose:

- create new sessions
- show active session overview
- alert on blocked, failed, done, and critical events
- link or jump to the session topic

Noise budget:

- very low

### Session topic

Purpose:

- show important events for one session
- show automation notes
- show escalations and outcomes
- preserve concise session history

Noise budget:

- moderate but curated

## Topic lifecycle

```text
General topic -> create session
  -> daemon allocates app session_id
  -> daemon creates session topic
  -> daemon launches run
  -> session topic becomes the room for that session
```

Session topics should not be deleted just because a run exits. The topic represents the app session, not the child process.

## Message classes

### General topic messages

- new session created
- session blocked
- session failed
- session done
- critical escalation required

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
