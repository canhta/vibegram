# Session Context

## Identity model

```text
Telegram session topic
   -> app session_id
      -> active run_id
         -> provider session id / process id / transcript path
```

This gives the product a stable identity even when the child process restarts or the provider resumes a session.

## Rolling snapshot

The snapshot should be compact and operational, not philosophical.

Recommended fields:

- `session_id`
- `run_id`
- `provider`
- `general_topic_id`
- `session_topic_id`
- `status`
- `phase`
- `last_goal`
- `last_question`
- `last_blocker`
- `recent_files_summary`
- `recent_tests_summary`
- `recent_events`
- `reply_attempt_count`
- `last_role_used`
- `escalation_state`
- `linked_decision_refs`
- `sandbox_profile`
- `pending_elevation`
- `evidence_refs`
- `owner_user_id`
- `last_human_actor_id`
- `delivery_state`

## Why not transcript-only

Transcript-only context makes every reply expensive and ambiguous:

- too much rereading
- too much provider-specific parsing at decision time
- harder to debug why the system replied the way it did
- easier to accidentally lose who approved what

## Long-term learning model

Long-term learning is file-based and explicit.

### Rules

Static role instructions:

- `memory/rules/global.md`
- `memory/rules/eng.md`
- `memory/rules/ceo.md`

### Decisions

Human corrections captured over time:

- append-only Markdown
- one decision per file or compact cluster
- reviewed and inspectable

### Promotions

Patterns promoted from repeated decisions after evals pass.

## Retrieval model

Use a local retrieval index with Markdown as the source of truth.

Recommended v1 shape:

- SQLite FTS for lexical retrieval
- simple metadata filters by role, trigger class, and provider
- optional rerank later

Not recommended for v1:

- embeddings-first retrieval
- remote vector services
- opaque provider-side memory

## Learning loop

```text
human teaches system
   -> decision markdown saved
   -> local index refreshed
   -> future retrieval improves
   -> promotion candidate identified
   -> eval passes
   -> rule promoted
```

The system should not auto-edit its own rules after one interaction.

For the current OpenAI-specific guidance that shaped this memory model, see [OpenAI Guidance](./openai-guidance.md).
For the trust and provenance model that shaped the snapshot, see [Trust Boundaries](./trust-boundaries.md).
