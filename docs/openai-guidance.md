# OpenAI Guidance

This file captures the current official OpenAI guidance that most directly affects the `ENG` and `CEO` role layer in `vibegram`.

## Model selection

As of the current official models page, OpenAI recommends:

- `gpt-5.4` as the flagship starting point for complex reasoning and coding
- `gpt-5.4-mini` or `gpt-5.4-nano` for lower-latency or lower-cost workloads

Source:
- [OpenAI models](https://developers.openai.com/api/docs/models)

Implications for `vibegram`:

- default high-trust path: `CEO` on `gpt-5.4`
- cost-sensitive fast path: `ENG` on `gpt-5.4-mini`
- actual defaults should still be measured by evals, not chosen once and forgotten

## Coding-specialized models

OpenAI also publishes coding-specialized GPT-5-family variants:

- `gpt-5-codex` is "a version of GPT-5 optimized for agentic coding tasks"
- `gpt-5.2-codex` is an upgraded coding model optimized for long-horizon agentic coding tasks

Sources:
- [GPT-5-Codex](https://developers.openai.com/api/docs/models/gpt-5-codex)
- [GPT-5.2-Codex](https://developers.openai.com/api/docs/models/gpt-5.2-codex)

Implications for `vibegram`:

- `ENG` may eventually benefit from a coding-optimized model
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

- `rules/global.md`, `rules/eng.md`, and `rules/ceo.md` should form a stable prompt prefix
- retrieved decisions and live session context should be appended after the stable rules
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

- `ENG` and `CEO` should return structured JSON like `reply | escalate | noop`
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

- do not use Background mode for quick `ENG` unblock replies
- consider it for slower `CEO` synthesis or heavier future workflows
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

## Recommended OpenAI stance

```text
App-owned memory is the truth
  + OpenAI Responses API for inference
  + structured outputs for role decisions
  + prompt caching via stable rule prefixes
  + optional conversation state as acceleration
  + evals before rule promotion
```

That is the cleanest way to stay aligned with OpenAI's current platform without making the product dependent on provider-side memory.
