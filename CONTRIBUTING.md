# Contributing to vibegram

Thanks for contributing.

`vibegram` is currently a design-first repo. The most important thing right now is to keep the written system coherent while the first implementation is being prepared.

## Before you start

Read these first:

1. [README.md](./README.md)
2. [AGENTS.md](./AGENTS.md)
3. [docs/README.md](./docs/README.md)
4. [docs/decisions.md](./docs/decisions.md)
5. [docs/implementation-plan.md](./docs/implementation-plan.md)
6. [docs/plans/README.md](./docs/plans/README.md)

## What good contributions look like right now

- clarify architecture
- tighten the implementation plan
- improve diagrams and schemas
- add research-backed constraints
- reduce ambiguity for future implementation

## What to avoid

- reintroducing terminal-mirror assumptions
- changing locked product decisions without updating `docs/decisions.md`
- inventing runtime commands or workflows that do not exist yet
- adding speculative code that ignores the current design set

## If you change the design

Update the affected files together:

- [docs/decisions.md](./docs/decisions.md) for any locked-decision change
- the relevant topic docs under [docs/](./docs)
- [docs/diagrams.md](./docs/diagrams.md) if flows or boundaries changed
- [docs/schemas/](./docs/schemas) if the data contract changed

## If you start implementation work

The current default implementation direction is:

- Go
- single module at repo root
- `cmd/vibegram/` plus `internal/` packages
- direct process runner first
- `systemd` as the default VPS story

Keep early implementation small and aligned with [docs/implementation-plan.md](./docs/implementation-plan.md).

## Pull requests

Good PRs are:

- small enough to review
- explicit about what changed
- linked to the affected design docs
- clear about whether they are docs-only or implementation work

Please include:

- what changed
- why it changed
- which docs or decisions were updated
- any follow-up work or open questions

If the work maps to a tracked phase plan, update the matching checklist in [docs/plans/](./docs/plans).
