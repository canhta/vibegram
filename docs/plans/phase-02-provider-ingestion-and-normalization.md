# Phase 2: Provider Ingestion and Normalization

## Goal

Turn provider-specific signals into one clean, deduped, provenance-aware event stream.

## Depends on

- [Phase 1: Foundation and Runtime](./phase-01-foundation-and-runtime.md)

## References

- [Provider Model](../provider-model.md)
- [Event Model](../event-model.md)
- [OpenAI Guidance](../openai-guidance.md)
- [Lessons from ccgram](../ccgram-lessons.md)
- [Trust Boundaries](../trust-boundaries.md)

## Deliverables

- Claude adapter
- Codex adapter
- normalized event types
- dedupe logic
- provenance on normalized events

## Checklist

### Provider adapters

- [x] Write parser fixtures for Claude hooks and transcripts
- [x] Write parser fixtures for Codex transcripts
- [x] Implement raw observation emission for Claude
- [x] Implement raw observation emission for Codex
- [x] Implement signal priority rules for both providers
- [x] Verify duplicate raw observations remain distinguishable before normalization

### Event normalization

- [x] Define the balanced normalized event set in code
- [x] Preserve trust provenance on normalized events
- [x] Write table-driven tests for all supported event mappings
- [x] Implement dedupe keys and replay protection
- [x] Verify one logical event becomes one normalized event
