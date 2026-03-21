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

- [ ] Write parser fixtures for Claude hooks and transcripts
- [ ] Write parser fixtures for Codex transcripts
- [ ] Implement raw observation emission for Claude
- [ ] Implement raw observation emission for Codex
- [ ] Implement signal priority rules for both providers
- [ ] Verify duplicate raw observations remain distinguishable before normalization

### Event normalization

- [ ] Define the balanced normalized event set in code
- [ ] Preserve trust provenance on normalized events
- [ ] Write table-driven tests for all supported event mappings
- [ ] Implement dedupe keys and replay protection
- [ ] Verify one logical event becomes one normalized event
