# Decision Memory Template

Use one file per durable human-taught decision.

```markdown
---
status: ACTIVE
role: ENG | CEO | BOTH
trigger: blocked | question | done | approval_needed | failed
provider: claude | codex | any
created_at: 2026-03-21T10:00:00Z
source_session_id: ses_...
source_run_id: run_...
---

# Decision: <short title>

## Situation

What the agent was asking or blocked on.

## Human decision

What the human chose.

## Why

Why this was the correct action.

## Good future behavior

How `vibegram` should act next time.

## Bad future behavior

What it should avoid doing.

## Example evidence

Short excerpt or summary of the triggering event.
```
