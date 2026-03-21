// Package claude implements the provider adapter for Claude Code.
//
// Claude Code emits signals via three sources in priority order:
//
//	hooks -> transcript -> PTY
//
// The adapter parses hook payloads (JSON sent to hook commands via stdin) and
// transcript lines (JSONL conversation messages) into raw observations that the
// normalizer can process.
package claude

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/providers"
)

// Adapter parses Claude Code signals into raw observations.
type Adapter struct {
	sessionID string
	runID     string
}

// New returns an Adapter for a specific app session and run.
func New(sessionID, runID string) *Adapter {
	return &Adapter{sessionID: sessionID, runID: runID}
}

// hookPayload is the JSON shape that Claude Code sends to hook commands via stdin.
type hookPayload struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	Message       string         `json:"message"`
	Timestamp     string         `json:"timestamp"`
	ToolInput     map[string]any `json:"tool_input"`
}

// ParseHook parses a Claude Code hook payload and returns a raw observation.
// Returns (observation, true, nil) when the hook produces a meaningful observation.
// Returns (zero, false, nil) when the hook event is not interesting.
// Returns (zero, false, err) when the payload is malformed.
func (a *Adapter) ParseHook(data []byte) (providers.RawObservation, bool, error) {
	var p hookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return providers.RawObservation{}, false, fmt.Errorf("claude hook: unmarshal: %w", err)
	}

	ts := parseTimestamp(p.Timestamp)

	switch p.HookEventName {
	case "PostToolUse":
		summary := fmt.Sprintf("Tool: %s", p.ToolName)
		if cmd, ok := p.ToolInput["command"].(string); ok && cmd != "" {
			summary = fmt.Sprintf("Tool: %s — %s", p.ToolName, truncate(cmd, 80))
		}
		return providers.RawObservation{
			Observation: events.Observation{
				SessionID:    a.sessionID,
				RunID:        a.runID,
				Provider:     events.ProviderClaude,
				Source:       events.SourceHook,
				RawType:      "tool_activity",
				RawTimestamp: ts,
				Summary:      summary,
				Details:      map[string]any{"tool": p.ToolName},
			},
		}, true, nil

	case "Stop":
		return providers.RawObservation{
			Observation: events.Observation{
				SessionID:    a.sessionID,
				RunID:        a.runID,
				Provider:     events.ProviderClaude,
				Source:       events.SourceHook,
				RawType:      "done",
				RawTimestamp: ts,
				Summary:      "Session completed",
			},
		}, true, nil

	case "Notification":
		rawType := classifyText(p.Message)
		if rawType == "" {
			return providers.RawObservation{}, false, nil
		}
		return providers.RawObservation{
			Observation: events.Observation{
				SessionID:    a.sessionID,
				RunID:        a.runID,
				Provider:     events.ProviderClaude,
				Source:       events.SourceHook,
				RawType:      rawType,
				RawTimestamp: ts,
				Summary:      p.Message,
			},
		}, true, nil

	default:
		return providers.RawObservation{}, false, nil
	}
}

// transcriptLine is the JSONL shape of a Claude Code conversation transcript.
type transcriptLine struct {
	Role    string            `json:"role"`
	Content []transcriptBlock `json:"content"`
}

type transcriptBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name"` // for tool_use
	Input map[string]any `json:"input"` // for tool_use
}

// ParseTranscriptLine parses one JSONL line from a Claude Code transcript.
// Returns (observation, true, nil) when the line produces a meaningful observation.
// Returns (zero, false, nil) when the line is not interesting.
// Returns (zero, false, err) when the line is malformed.
func (a *Adapter) ParseTranscriptLine(ts time.Time, data []byte) (providers.RawObservation, bool, error) {
	var line transcriptLine
	if err := json.Unmarshal(data, &line); err != nil {
		return providers.RawObservation{}, false, fmt.Errorf("claude transcript: unmarshal: %w", err)
	}

	if line.Role != "assistant" {
		return providers.RawObservation{}, false, nil
	}

	// Check for tool_use blocks first — higher signal than text.
	for _, block := range line.Content {
		if block.Type == "tool_use" {
			summary := fmt.Sprintf("Tool: %s", block.Name)
			if cmd, ok := block.Input["command"].(string); ok && cmd != "" {
				summary = fmt.Sprintf("Tool: %s — %s", block.Name, truncate(cmd, 80))
			}
			return providers.RawObservation{
				Observation: events.Observation{
					SessionID:    a.sessionID,
					RunID:        a.runID,
					Provider:     events.ProviderClaude,
					Source:       events.SourceTranscript,
					RawType:      "tool_activity",
					RawTimestamp: ts,
					Summary:      summary,
					Details:      map[string]any{"tool": block.Name},
				},
			}, true, nil
		}
	}

	// No tool_use — check the text content for question or blocked signals.
	for _, block := range line.Content {
		if block.Type == "text" && block.Text != "" {
			rawType := classifyText(block.Text)
			if rawType != "" {
				return providers.RawObservation{
					Observation: events.Observation{
						SessionID:    a.sessionID,
						RunID:        a.runID,
						Provider:     events.ProviderClaude,
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
		"need clarification", "i need clarification",
	}
	for _, phrase := range questionPhrases {
		if strings.Contains(lower, phrase) {
			return "question"
		}
	}

	return ""
}

func parseTimestamp(raw string) time.Time {
	if raw == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
