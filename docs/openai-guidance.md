# OpenAI Guidance

This file captures the current official OpenAI guidance that most directly affects the support-role layer in `vibegram`.
The current executable slice uses one bounded support role; future specialized profiles may still split that behavior later.

## Model selection

As of the current official models page, OpenAI recommends:

- `gpt-5.4` as the flagship starting point for complex reasoning and coding
- `gpt-5.4-mini` or `gpt-5.4-nano` for lower-latency or lower-cost workloads

Source:
- [OpenAI models](https://developers.openai.com/api/docs/models)

Implications for `vibegram`:

- default high-trust path: support role on `gpt-5.4`
- cost-sensitive fast path: support role on `gpt-5.4-mini`
- actual defaults should still be measured by evals, not chosen once and forgotten

## Coding-specialized models

OpenAI also publishes coding-specialized GPT-5-family variants:

- `gpt-5-codex` is "a version of GPT-5 optimized for agentic coding tasks"
- `gpt-5.2-codex` is an upgraded coding model optimized for long-horizon agentic coding tasks

Sources:
- [GPT-5-Codex](https://developers.openai.com/api/docs/models/gpt-5-codex)
- [GPT-5.2-Codex](https://developers.openai.com/api/docs/models/gpt-5.2-codex)

Implications for `vibegram`:

- the support role may eventually benefit from a coding-optimized model
- the daemon should hide model choice behind a clean role executor interface
- the docs should keep "GPT-5 family" as the product contract, not one brittle model ID

## Conversation state

OpenAI offers two main server-side conversation-state mechanisms:

1. `previous_response_id` chains responses together
2. the Conversations API persists a long-running conversation object with its own durable ID

Official guidance:

- the Conversations API works with Responses to persist state across sessions, devices, or jobs
- `previous_response_id` lets you chain responses into a threaded conversation
- response objects are saved for 30 days by default unless `store=false`
- conversation objects and their items are not subject to that 30-day TTL

Source:
- [Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)

Implications for `vibegram`:

- provider-side conversation state is useful
- it should not be the source of truth for learned behavior or policy
- local Markdown memory remains the right primary memory layer

## Prompt caching

OpenAI's prompt caching guidance strongly matches the `vibegram` role design:

- cache hits require exact prefix matches
- static instructions and examples should be placed first
- variable session context should be appended later

Source:
- [Prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching)

Implications for `vibegram`:

- the current hardcoded ruleset should stay as a stable prompt prefix
- later `memory/rules/*.md` and retrieved decisions can be appended after the stable rules
- role prompts should change slowly and intentionally

## Structured outputs

OpenAI's structured-output guidance supports exactly the behavior `vibegram` needs:

- use `json_schema`
- use `strict: true`
- name keys clearly
- add titles and descriptions where helpful
- use evals to improve the schema over time

Source:
- [Structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs)

Implications for `vibegram`:

- the support role should return structured JSON like `reply | escalate | noop`
- malformed or incomplete role output should fail closed
- schema shape is part of the safety contract

## Background mode

OpenAI's Background mode is useful, but not for everything:

- it runs long tasks asynchronously
- developers poll the response status over time
- it stores response data for roughly 10 minutes to enable polling
- it is not Zero Data Retention compatible

Source:
- [Background mode](https://developers.openai.com/api/docs/guides/background)

Implications for `vibegram`:

- do not use Background mode for quick support-role unblock replies
- consider it only for slower future workflows outside the core correctness path
- do not make Background mode part of the core correctness path

## Evals and graders

OpenAI's eval guidance reinforces the same direction we already chose:

- use human-labeled reference outputs where possible
- use graders to compare model output against the intended behavior
- evolve prompts and schemas through evals, not hidden self-mutation

Source:
- [Working with evals](https://developers.openai.com/api/docs/guides/evals)

Implications for `vibegram`:

- every promoted human-taught decision should pass through evals before becoming a rule
- the reply-safety gate should be a first-class release check

## Prompt injection and sandboxing

Recent OpenAI safety guidance makes three points especially relevant to `vibegram`:

1. prompt injection is an evolving security problem and should be treated as a real trust-boundary issue, not just an input-filtering bug
2. agents should get only the sensitive access they actually need
3. sandboxing and confirmation for consequential actions are key defenses

OpenAI's current public guidance explicitly recommends limiting access to sensitive data and carefully reviewing consequential actions. Its Codex safety material also emphasizes sandboxing, network-disabled defaults, and workspace-scoped file access as core mitigations.

Sources:

- [Understanding prompt injections](https://openai.com/index/prompt-injections/)
- [Designing AI agents to resist prompt injection](https://openai.com/index/designing-agents-to-resist-prompt-injection/)
- [Introducing upgrades to Codex](https://openai.com/index/introducing-upgrades-to-codex/)
- [Codex system card addendum](https://cdn.openai.com/pdf/8df7697b-c1b2-4222-be00-1fd3298f351d/codex_system_card.pdf)
- [GPT-5.3-Codex system card](https://deploymentsafety.openai.com/gpt-5-3-codex/gpt-5-3-codex.pdf)

Implications for `vibegram`:

- untrusted transcript or tool content must never become policy
- the runner should enforce sandbox profiles
- network should be disabled by default
- higher-risk actions should require explicit approval
- role outputs should be bounded and structured, not open-ended authority

## Recommended OpenAI stance

```text
App-owned memory is the truth
  + OpenAI Responses API for inference
  + structured outputs for role decisions
  + prompt caching via stable rule prefixes
  + optional conversation state as acceleration
  + least-privilege execution around the model
  + evals before rule promotion
```

That is the cleanest way to stay aligned with OpenAI's current platform without making the product dependent on provider-side memory.

For the repo-level design correction that follows from this, see [Trust Boundaries](./trust-boundaries.md).
