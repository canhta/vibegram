# Contributing to vibegram

Thanks for contributing.

`vibegram` is currently a design-first repo. The most important thing right now is to keep the written system coherent while the first implementation is being prepared.

## Before you start

Read these first:

1. [README.md](./README.md)
2. [AGENTS.md](./AGENTS.md)
3. [docs/decisions.md](./docs/decisions.md)
4. [docs/architecture.md](./docs/architecture.md)
5. [docs/runtime-ops.md](./docs/runtime-ops.md)

## What good contributions look like right now

- clarify architecture
- reduce duplicated documentation
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
- the relevant surviving source-of-truth docs under [docs/](./docs)

## If you start implementation work

The current default implementation direction is:

- Go
- single module at repo root
- `cmd/vibegram/` plus `internal/` packages
- direct process runner first
- `systemd` as the default VPS story

Keep early implementation small and aligned with the locked product shape in [docs/decisions.md](./docs/decisions.md), [docs/architecture.md](./docs/architecture.md), and [docs/runtime-ops.md](./docs/runtime-ops.md).

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
