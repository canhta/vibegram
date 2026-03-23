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
	Item      transcriptEntry `json:"item"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type transcriptEntry struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Text             string `json:"text"`
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code"`
	Status           string `json:"status"`
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
		summary, ok := summarizeFunctionCall(item)
		if !ok {
			return providers.RawObservation{}, false, nil
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
				if isMetaAgentMessageNoise(block.Text) {
					return providers.RawObservation{}, false, nil
				}
				rawType := ClassifyText(block.Text)
				if rawType != "" {
					return providers.RawObservation{
						Observation: events.Observation{
							SessionID:    a.sessionID,
							RunID:        a.runID,
							Provider:     events.ProviderCodex,
							Source:       events.SourceTranscript,
							RawType:      rawType,
							RawTimestamp: ts,
							Summary:      summarizeAgentText(block.Text, rawType),
						},
					}, true, nil
				}
			}
		}
		return providers.RawObservation{}, false, nil

	case "item.completed":
		switch item.Item.Type {
		case "command_execution":
			summary, ok := summarizeCommandExecution(item.Item.Command)
			if !ok {
				return providers.RawObservation{}, false, nil
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
					Details:      map[string]any{"tool": "shell"},
					ProviderID:   item.Item.ID,
				},
			}, true, nil

		case "agent_message":
			text := strings.TrimSpace(item.Item.Text)
			if isMetaAgentMessageNoise(text) {
				return providers.RawObservation{}, false, nil
			}
			rawType := ClassifyText(text)
			if rawType == "" {
				return providers.RawObservation{}, false, nil
			}
			return providers.RawObservation{
				Observation: events.Observation{
					SessionID:    a.sessionID,
					RunID:        a.runID,
					Provider:     events.ProviderCodex,
					Source:       events.SourceTranscript,
					RawType:      rawType,
					RawTimestamp: ts,
					Summary:      summarizeAgentText(text, rawType),
					ProviderID:   item.Item.ID,
				},
			}, true, nil
		}
		return providers.RawObservation{}, false, nil

	default:
		return providers.RawObservation{}, false, nil
	}
}

// classifyText returns the raw event type for a text string, or "" if not interesting.
func ClassifyText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}

	approvalPhrases := []string{
		"reply `approve`",
		"reply approve",
		"say `approve`",
		"say approve",
		"approve and i'll create it",
		"approve and i’ll create it",
		"once you approve",
		"if you approve",
		"need your approval",
	}
	for _, phrase := range approvalPhrases {
		if strings.Contains(lower, phrase) {
			return "approval_needed"
		}
	}

	blockerResolvedPhrases := []string{
		"resolved the blocker",
		"blocker is resolved",
		"blocker was resolved",
		"i'm unblocked",
		"i am unblocked",
		"unblocked now",
	}
	for _, phrase := range blockerResolvedPhrases {
		if strings.Contains(lower, phrase) {
			return "blocker_resolved"
		}
	}
	if strings.Contains(lower, "can continue now") && (strings.Contains(lower, "resolved") || strings.Contains(lower, "fixed")) {
		return "blocker_resolved"
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
	if strings.Contains(lower, "?") {
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

func summarizeAgentText(text, rawType string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	if rawType == "question" {
		return text
	}
	return truncate(text, 200)
}

func isMetaAgentMessageNoise(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	return strings.Contains(lower, "some of what we're working on might be easier to explain if i can show it to you in a web browser") &&
		strings.Contains(lower, "want to try it?")
}

func summarizeFunctionCall(item transcriptItem) (string, bool) {
	summary := fmt.Sprintf("Tool: %s", item.Name)
	if item.Arguments == "" {
		return summary, true
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
		return summary, true
	}

	cmd, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmd) == "" {
		return summary, true
	}
	return summarizeCommandExecution(cmd)
}

func summarizeCommandExecution(command string) (string, bool) {
	payload := normalizeShellCommand(command)
	if payload == "" {
		return "", false
	}
	if isReadOnlyNoise(payload) {
		return "", false
	}
	if !isMeaningfulCommand(payload) {
		return "", false
	}
	return fmt.Sprintf("Tool: shell — %s", truncate(payload, 120)), true
}

func normalizeShellCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	for _, marker := range []string{" -lc ", " -c "} {
		if idx := strings.Index(command, marker); idx >= 0 {
			command = strings.TrimSpace(command[idx+len(marker):])
			break
		}
	}
	return trimShellQuotes(command)
}

func trimShellQuotes(command string) string {
	command = strings.TrimSpace(command)
	if len(command) >= 2 {
		if (command[0] == '"' && command[len(command)-1] == '"') || (command[0] == '\'' && command[len(command)-1] == '\'') {
			return strings.TrimSpace(command[1 : len(command)-1])
		}
	}
	return command
}

func isReadOnlyNoise(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return true
	}

	readPrefixes := []string{
		"cat ", "sed ", "rg ", "grep ", "find ", "pwd", "ls", "git log", "git status",
		"git diff", "git show", "head ", "tail ", "wc ", "which ", "readlink ", "stat ",
	}
	for _, prefix := range readPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	if strings.Contains(lower, "/skills/") || strings.Contains(lower, "skill.md") {
		return true
	}
	return false
}

func isMeaningfulCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}

	meaningfulTokens := []string{
		"apply_patch",
		" test", "test ", " verify", "verify ", " lint", "lint ", " check", "check ",
		"build", " run ", " run", "start", "serve", "dev", "preview",
		"install", " add ", "add ", " update", "upgrade", "get ",
		"mkdir", "touch", "mv ", "cp ", "rm ", "chmod", "chown", "patch", "git apply",
		"git commit", "git checkout", "git cherry-pick", "git merge", "git rebase",
	}
	for _, token := range meaningfulTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}

	return strings.Contains(command, ">") || strings.Contains(command, ">>") || strings.Contains(lower, "tee ")
}
