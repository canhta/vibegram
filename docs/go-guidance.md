# Go Guidance

This file captures the official Go guidance that most directly affects how `vibegram` should be built.

## Why Go still looks like the right fit

`vibegram` is a long-running daemon with:

- local process management
- concurrent watchers
- clear state boundaries
- strong testing needs
- simple deployment goals

Go fits that shape well, and the official docs line up with the repo layout we already planned.

## Project layout

Go's official module layout guidance supports the structure we want:

- use a single module rooted at the repository
- match the `module` path in `go.mod` to the repository path
- start simple, then split into supporting packages
- prefer `internal/` for non-public implementation packages

Official guidance:

- "Larger packages or commands may benefit from splitting off some functionality into supporting packages. Initially, it’s recommended placing such packages into a directory named `internal`."

Source:
- [Organizing a Go module](https://go.dev/doc/modules/layout)

Implications for `vibegram`:

- one module is enough for v1
- `internal/runner`, `internal/events`, `internal/state`, `internal/policy`, and `internal/telegram` are the right default shape
- we should avoid exporting internal orchestration packages too early

## Context propagation

The official `context` package guidance is directly relevant for:

- daemon shutdown
- run cancellation
- provider watchers
- network calls

Official guidance:

- pass `Context` explicitly
- make it the first parameter, usually `ctx`
- do not store `Context` inside structs
- always call the returned cancel function, otherwise the child context leaks

Source:
- [context package docs](https://pkg.go.dev/context)

Implications for `vibegram`:

- every long-running watcher should accept `ctx`
- process lifecycle should be driven by context cancellation
- the app should be able to stop a run, stop all watchers, and shut down cleanly

## Process execution and safety

The `os/exec` package docs make two things especially important:

1. `os/exec` does not invoke a shell by default
2. implicit execution from the current directory is intentionally blocked for security

Official guidance:

- `os/exec` "intentionally does not invoke the system shell"
- use `exec.CommandContext` when you want context cancellation to interrupt the process
- current-directory executable resolution is considered a security risk and returns `ErrDot`

Source:
- [os/exec package docs](https://pkg.go.dev/os/exec)

Implications for `vibegram`:

- the direct runner should use `exec.CommandContext`
- provider launch commands should be resolved explicitly, not through fragile shell strings
- if the product ever supports user-supplied provider paths, those paths should be explicit and validated

## Error handling

Official Go guidance is clear:

- return errors as values
- use `%w` only when you want to expose that wrapped error to callers
- use `errors.Is` and `errors.As` to inspect error trees
- wrapping makes the wrapped error part of your API contract

Sources:
- [errors package](https://pkg.go.dev/errors)
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)

Implications for `vibegram`:

- provider-facing low-level errors can be wrapped when callers need to act on them
- internal implementation errors should not always be exposed through `%w`
- the public package contract should define which sentinels or types callers may rely on

## Testing and fuzzing

The official docs make fuzzing a strong fit for `vibegram` because it will parse:

- transcripts
- PTY output
- hook payloads
- normalized event mappings

Official guidance:

- normal tests run with `go test`
- fuzz tests use `func FuzzXxx(*testing.F)`
- fuzz inputs can be seeded with `f.Add`
- fuzzing is run via `go test -fuzz=...`

Sources:
- [Add a test](https://go.dev/doc/tutorial/add-a-test)
- [Getting started with fuzzing](https://go.dev/doc/tutorial/fuzz)

Implications for `vibegram`:

- unit tests should cover state, routing, and policy
- fuzz tests should target transcript parsing, event normalization, and Telegram rendering boundaries

## Recommended Go-specific stance

```text
One module
  -> internal packages by responsibility
  -> context-driven lifecycle
  -> exec.CommandContext for provider runs
  -> explicit error contracts
  -> go test first, fuzz parsers second
```

This is the most official, boring, and maintainable way to build the daemon.
