package claude_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/providers/claude"
)

func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "fixtures", "claude")
}

func TestParseHookPostToolUse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixturesDir(), "hook_post_tool_use.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseHook(data)
	if err != nil {
		t.Fatalf("ParseHook() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseHook() ok = false, want true")
	}

	if obs.Provider != events.ProviderClaude {
		t.Errorf("Provider = %q, want %q", obs.Provider, events.ProviderClaude)
	}
	if obs.Source != events.SourceHook {
		t.Errorf("Source = %q, want %q", obs.Source, events.SourceHook)
	}
	if obs.RawType != "tool_activity" {
		t.Errorf("RawType = %q, want %q", obs.RawType, "tool_activity")
	}
	if obs.SessionID != "ses_001" {
		t.Errorf("SessionID = %q, want ses_001", obs.SessionID)
	}
	if obs.RunID != "run_001" {
		t.Errorf("RunID = %q, want run_001", obs.RunID)
	}
	if obs.Summary == "" {
		t.Error("Summary is empty")
	}
}

func TestParseHookStop(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixturesDir(), "hook_stop.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseHook(data)
	if err != nil {
		t.Fatalf("ParseHook() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseHook() ok = false, want true")
	}
	if obs.RawType != "done" {
		t.Errorf("RawType = %q, want done", obs.RawType)
	}
}

func TestParseHookNotificationQuestion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixturesDir(), "hook_notification_question.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseHook(data)
	if err != nil {
		t.Fatalf("ParseHook() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseHook() ok = false, want true")
	}
	if obs.RawType != "question" {
		t.Errorf("RawType = %q, want question", obs.RawType)
	}
}

func TestParseHookUnknownEventIsIgnored(t *testing.T) {
	a := claude.New("ses_001", "run_001")
	_, ok, err := a.ParseHook([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","timestamp":"2026-03-21T10:00:00Z"}`))
	if err != nil {
		t.Fatalf("ParseHook() error = %v", err)
	}
	if ok {
		t.Fatal("ParseHook() ok = true, want false for unknown event")
	}
}

func TestParseHookMalformedJSON(t *testing.T) {
	a := claude.New("ses_001", "run_001")
	_, _, err := a.ParseHook([]byte(`not json`))
	if err == nil {
		t.Fatal("ParseHook() error = nil, want error for malformed JSON")
	}
}

func TestParseTranscriptLineToolUse(t *testing.T) {
	line := []byte(`{"role":"assistant","content":[{"type":"text","text":"I will check the files."},{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"ls -la"}}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseTranscriptLine() ok = false, want true")
	}
	if obs.RawType != "tool_activity" {
		t.Errorf("RawType = %q, want tool_activity", obs.RawType)
	}
	if obs.Source != events.SourceTranscript {
		t.Errorf("Source = %q, want transcript", obs.Source)
	}
}

func TestParseTranscriptLineQuestion(t *testing.T) {
	line := []byte(`{"role":"assistant","content":[{"type":"text","text":"Should I add error handling for the nil case?"}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseTranscriptLine() ok = false, want true")
	}
	if obs.RawType != "question" {
		t.Errorf("RawType = %q, want question", obs.RawType)
	}
}

func TestParseTranscriptLineBlocked(t *testing.T) {
	line := []byte(`{"role":"assistant","content":[{"type":"text","text":"I'm unable to proceed because the required API key is not set."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := claude.New("ses_001", "run_001")
	obs, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseTranscriptLine() ok = false, want true")
	}
	if obs.RawType != "blocked" {
		t.Errorf("RawType = %q, want blocked", obs.RawType)
	}
}

func TestParseTranscriptLineUserTurnIsIgnored(t *testing.T) {
	line := []byte(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","content":"output"}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := claude.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for user turn")
	}
}

func TestParseTranscriptLineBoringTextIsIgnored(t *testing.T) {
	line := []byte(`{"role":"assistant","content":[{"type":"text","text":"I have completed the task successfully."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := claude.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for boring text")
	}
}

func TestParseTranscriptFixture(t *testing.T) {
	lines, err := os.ReadFile(filepath.Join(fixturesDir(), "transcript.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	a := claude.New("ses_001", "run_001")
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	var got []string
	for _, raw := range splitLines(lines) {
		obs, ok, err := a.ParseTranscriptLine(ts, raw)
		if err != nil {
			t.Fatalf("ParseTranscriptLine() error = %v for line %q", err, raw)
		}
		if ok {
			got = append(got, obs.RawType)
		}
	}

	want := []string{"tool_activity", "question", "blocked"}
	if len(got) != len(want) {
		t.Fatalf("got %v observations, want %v", got, want)
	}
	for i, rawType := range want {
		if got[i] != rawType {
			t.Errorf("obs[%d].RawType = %q, want %q", i, got[i], rawType)
		}
	}
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for _, line := range splitByNewline(data) {
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitByNewline(data []byte) [][]byte {
	var result [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			result = append(result, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		result = append(result, data[start:])
	}
	return result
}
