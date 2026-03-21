# Provider Model

## Supported providers in v1

- Claude Code
- Codex

## Capability reality

The providers are not symmetric.

### Claude Code

Strengths:

- hooks
- structured transcript
- clear lifecycle signals

Recommended signal priority:

```text
hooks -> transcript -> PTY fallback
```

### Codex

Strengths:

- structured transcript
- interactive session support
- resumable workflows

Weakness:

- no hook channel in the current model we are designing around

Recommended signal priority:

```text
transcript -> PTY fallback
```

## Adapter contract

Every provider adapter should produce raw observations in the same shape:

```text
observation {
  provider
  run_id
  source        // hook | transcript | pty
  raw_type
  raw_timestamp
  payload
}
```

The adapter does not decide whether something is a `blocked` event or a `done` event for the product. It only makes raw observations legible enough for the normalizer.

## Why adapters matter

Without a hard adapter boundary, provider quirks leak upward and the rest of the system becomes provider-specific.

## Resume and recovery

The provider adapter should expose:

- fresh launch
- continue last
- resume by provider session ID when available
- health probe
- transcript discovery rules

But the app session remains the top-level identity.

## Future providers

Additional providers should be accepted only if they can satisfy:

- structured transcript or equivalent
- detectable lifecycle
- stable stdin/stdout or PTY behavior
- predictable resume semantics
