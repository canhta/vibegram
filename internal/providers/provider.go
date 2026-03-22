package providers

import (
	"fmt"

	"github.com/canhta/vibegram/internal/events"
)

type RawObservation struct {
	events.Observation
}

type SessionResult struct {
	ProviderSessionID string
	Message           string
	RawOutput         string
}

func (r RawObservation) IdentityKey() string {
	return fmt.Sprintf(
		"%s:%s:%s:%s:%s:%s",
		r.Provider,
		r.RunID,
		r.Source,
		r.RawType,
		r.ProviderID,
		r.RawTimestamp.UTC().Format("20060102T150405.000000000Z"),
	)
}

type PriorityPicker struct {
	provider events.Provider
}

func NewPriorityPicker(provider events.Provider) PriorityPicker {
	return PriorityPicker{provider: provider}
}

func (p PriorityPicker) Preferred(observations []events.Observation) (events.Observation, bool) {
	if len(observations) == 0 {
		return events.Observation{}, false
	}

	best := observations[0]
	bestRank := p.rank(best.Source)
	for _, observation := range observations[1:] {
		rank := p.rank(observation.Source)
		if rank < bestRank {
			best = observation
			bestRank = rank
		}
	}

	return best, true
}

func (p PriorityPicker) rank(source events.Source) int {
	switch p.provider {
	case events.ProviderClaude:
		switch source {
		case events.SourceHook:
			return 0
		case events.SourceTranscript:
			return 1
		case events.SourcePTY:
			return 2
		default:
			return 3
		}
	case events.ProviderCodex:
		switch source {
		case events.SourceTranscript:
			return 0
		case events.SourcePTY:
			return 1
		default:
			return 2
		}
	default:
		return 3
	}
}
