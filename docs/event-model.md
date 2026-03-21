# Event Model

## Goal

The event model should be small, stable, and expressive enough to power:

- Telegram rendering
- session snapshot updates
- automation policy
- eval fixtures

## Balanced event set

- `session_started`
- `phase_changed`
- `tool_activity`
- `files_changed`
- `tests_changed`
- `question`
- `blocked`
- `approval_needed`
- `agent_reply_sent`
- `done`
- `failed`

## Event routing rules

```text
phase_changed / files_changed / tests_changed / tool_activity
  -> session topic only

question / blocked
  -> session topic
  -> policy engine may auto-reply or escalate

approval_needed / failed
  -> session topic
  -> General topic
  -> human escalation

done
  -> session topic summary
  -> General topic completion alert
```

## Event payload guidelines

Every normalized event should preserve provenance.

Minimum v1 field:

- `source_class`: `trusted_policy` | `trusted_system` | `untrusted_evidence`
- `delivery_key`: stable idempotency key for user-visible rendering
- `artifact_refs`: optional references to stored evidence blobs or snippets

### `phase_changed`

Use when the work meaningfully changes phase:

- planning
- reading code
- editing
- running tests
- waiting

### `tool_activity`

Should be grouped and summarized. This is not raw tool echo.

Example:

```text
Ran 3 commands, edited 2 files
```

### `files_changed`

Keep compact:

- file count
- top files
- optional category tags

### `tests_changed`

Keep compact:

- test command label
- pass/fail
- count summary
- one or two failing cases if useful

## Example normalized event

```json
{
  "event_id": "evt_01JXYZ",
  "session_id": "ses_01JXYZ",
  "run_id": "run_01JXYZ",
  "provider": "codex",
  "event_type": "blocked",
  "source_class": "untrusted_evidence",
  "delivery_key": "ses_01JXYZ:blocked:abc123",
  "severity": "warning",
  "timestamp": "2026-03-21T10:00:00Z",
  "summary": "Agent is blocked on a missing environment variable.",
  "details": {
    "question": "Should I use STAGING_API_KEY or create a new token?",
    "confidence": "medium",
    "safe_auto_reply": false
  },
  "sources": [
    {
      "source": "transcript",
      "provider_type": "response_item",
      "provider_id": "abc123"
    }
  ],
  "artifact_refs": ["art_evt_01JXYZ"]
}
```

## Telegram rendering rules

- one event should produce at most one primary visible message
- renderer text is based on normalized `summary` and selected details
- raw evidence should remain available for future artifact work, not as default output
- renderers should use `delivery_key` or equivalent idempotency state to avoid duplicate visible sends
