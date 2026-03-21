package codex_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/providers/codex"
)

func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "fixtures", "codex")
}

func TestParseTranscriptLineFunctionCall(t *testing.T) {
	line := []byte(`{"type":"function_call","name":"shell","call_id":"call_001","arguments":"{\"command\":\"ls -la\"}"}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
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
	if obs.Provider != events.ProviderCodex {
		t.Errorf("Provider = %q, want codex", obs.Provider)
	}
	if obs.ProviderID != "call_001" {
		t.Errorf("ProviderID = %q, want call_001", obs.ProviderID)
	}
	if obs.SessionID != "ses_001" {
		t.Errorf("SessionID = %q, want ses_001", obs.SessionID)
	}
}

func TestParseTranscriptLineQuestion(t *testing.T) {
	line := []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Which test framework should I use for this project?"}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
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
	line := []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I'm stuck because the test command is failing with a permission error."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
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

func TestParseTranscriptLineFunctionCallOutputIsIgnored(t *testing.T) {
	line := []byte(`{"type":"function_call_output","call_id":"call_001","output":"total 8\n..."}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for function_call_output")
	}
}

func TestParseTranscriptLineUserMessageIsIgnored(t *testing.T) {
	line := []byte(`{"type":"message","role":"user","content":[{"type":"input_text","text":"Please check the files."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for user message")
	}
}

func TestParseTranscriptLineBoringTextIsIgnored(t *testing.T) {
	line := []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I have completed the task successfully."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for boring text")
	}
}

func TestParseTranscriptMalformedJSON(t *testing.T) {
	a := codex.New("ses_001", "run_001")
	_, _, err := a.ParseTranscriptLine(time.Now(), []byte(`not json`))
	if err == nil {
		t.Fatal("ParseTranscriptLine() error = nil, want error for malformed JSON")
	}
}

func TestParseTranscriptFixture(t *testing.T) {
	lines, err := os.ReadFile(filepath.Join(fixturesDir(), "transcript.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	a := codex.New("ses_001", "run_001")
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
