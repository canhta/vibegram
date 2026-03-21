// Package codex implements the provider adapter for OpenAI Codex.
//
// Codex emits signals via two sources in priority order:
//
//	transcript -> PTY
//
// The adapter parses transcript lines (JSONL output items in the OpenAI
// Responses API format) into raw observations that the normalizer can process.
package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/providers"
)

// Adapter parses Codex signals into raw observations.
type Adapter struct {
	sessionID string
	runID     string
}

// New returns an Adapter for a specific app session and run.
func New(sessionID, runID string) *Adapter {
	return &Adapter{sessionID: sessionID, runID: runID}
}

// transcriptItem is the JSONL shape of one item in a Codex Responses API output stream.
type transcriptItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   []contentBlock  `json:"content"`
	Name      string          `json:"name"`      // for function_call
	CallID    string          `json:"call_id"`   // for function_call
	Arguments string          `json:"arguments"` // for function_call (JSON string)
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseTranscriptLine parses one JSONL line from a Codex transcript.
// Returns (observation, true, nil) when the line produces a meaningful observation.
// Returns (zero, false, nil) when the line is not interesting.
// Returns (zero, false, err) when the line is malformed.
func (a *Adapter) ParseTranscriptLine(ts time.Time, data []byte) (providers.RawObservation, bool, error) {
	var item transcriptItem
	if err := json.Unmarshal(data, &item); err != nil {
		return providers.RawObservation{}, false, fmt.Errorf("codex transcript: unmarshal: %w", err)
	}

	switch item.Type {
	case "function_call":
		summary := fmt.Sprintf("Tool: %s", item.Name)
		if item.Arguments != "" {
			var args map[string]any
			if err := json.Unmarshal([]byte(item.Arguments), &args); err == nil {
				if cmd, ok := args["command"].(string); ok && cmd != "" {
					summary = fmt.Sprintf("Tool: %s — %s", item.Name, truncate(cmd, 80))
				}
			}
		}
		return providers.RawObservation{
			Observation: events.Observation{
				SessionID:    a.sessionID,
				RunID:        a.runID,
				Provider:     events.ProviderCodex,
				Source:       events.SourceTranscript,
				RawType:      "tool_activity",
				RawTimestamp: ts,
				Summary:      summary,
				Details:      map[string]any{"tool": item.Name},
				ProviderID:   item.CallID,
			},
		}, true, nil

	case "message":
		if item.Role != "assistant" {
			return providers.RawObservation{}, false, nil
		}
		for _, block := range item.Content {
			if block.Type == "output_text" && block.Text != "" {
				rawType := classifyText(block.Text)
				if rawType != "" {
					return providers.RawObservation{
						Observation: events.Observation{
							SessionID:    a.sessionID,
							RunID:        a.runID,
							Provider:     events.ProviderCodex,
							Source:       events.SourceTranscript,
							RawType:      rawType,
							RawTimestamp: ts,
							Summary:      truncate(block.Text, 200),
						},
					}, true, nil
				}
			}
		}
		return providers.RawObservation{}, false, nil

	default:
		return providers.RawObservation{}, false, nil
	}
}

// classifyText returns the raw event type for a text string, or "" if not interesting.
func classifyText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}

	blockedPhrases := []string{
		"i'm stuck", "i am stuck",
		"i'm blocked", "i am blocked",
		"i can't proceed", "i cannot proceed",
		"i'm unable", "i am unable",
		"i don't have access", "i do not have access",
		"i need clarification",
		"i need more information to proceed",
		"permission denied",
	}
	for _, phrase := range blockedPhrases {
		if strings.Contains(lower, phrase) {
			return "blocked"
		}
	}

	if strings.HasSuffix(lower, "?") {
		return "question"
	}

	questionPhrases := []string{
		"should i ", "do you want me to", "which ",
		"what would you like", "how should i",
		"can you clarify", "could you clarify",
		"need clarification",
	}
	for _, phrase := range questionPhrases {
		if strings.Contains(lower, phrase) {
			return "question"
		}
	}

	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
