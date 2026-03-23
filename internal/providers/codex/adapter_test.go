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
	line := []byte(`{"type":"function_call","name":"shell","call_id":"call_001","arguments":"{\"command\":\"go test ./...\"}"}`)
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
	if obs.Summary != "Tool: shell — go test ./..." {
		t.Errorf("Summary = %q, want Tool: shell — go test ./...", obs.Summary)
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

func TestParseTranscriptLineBlockerResolved(t *testing.T) {
	line := []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I resolved the blocker by adding the missing API token, and I can continue now."}]}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	obs, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseTranscriptLine() ok = false, want true")
	}
	if obs.RawType != "blocker_resolved" {
		t.Errorf("RawType = %q, want blocker_resolved", obs.RawType)
	}
}

func TestParseTranscriptLineCommandExecutionCompleted(t *testing.T) {
	line := []byte(`{"type":"item.completed","item":{"id":"item_3","type":"command_execution","command":"/bin/zsh -lc pwd","aggregated_output":"/tmp/project\n","exit_code":0,"status":"completed"}}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for read-only noise")
	}
}

func TestParseTranscriptLineCommandExecutionShowsHumanReadableTests(t *testing.T) {
	line := []byte(`{"type":"item.completed","item":{"id":"item_3","type":"command_execution","command":"/bin/zsh -lc \"go test ./...\"","aggregated_output":"","exit_code":0,"status":"completed"}}`)
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
	if obs.Summary != "Tool: shell — go test ./..." {
		t.Errorf("Summary = %q", obs.Summary)
	}
}

func TestParseTranscriptLineCommandExecutionIgnoresSkillReads(t *testing.T) {
	line := []byte(`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc \"sed -n '1,220p' /Users/canh/.codex/superpowers/skills/using-superpowers/SKILL.md\"","aggregated_output":"","exit_code":0,"status":"completed"}}`)
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for skill read noise")
	}
}

func TestParseTranscriptLineAgentMessageQuestion(t *testing.T) {
	line := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_4\",\"type\":\"agent_message\",\"text\":\"`pwd` returned `/tmp/project`.\\n\\nWhich framework do you want to use?\"}}")
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
	if obs.ProviderID != "item_4" {
		t.Errorf("ProviderID = %q, want item_4", obs.ProviderID)
	}
	if obs.Summary != "`pwd` returned `/tmp/project`.\n\nWhich framework do you want to use?" {
		t.Errorf("Summary = %q", obs.Summary)
	}
}

func TestParseTranscriptLineAgentMessageBrowserOfferIsIgnored(t *testing.T) {
	line := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_5\",\"type\":\"agent_message\",\"text\":\"Some of what we're working on might be easier to explain if I can show it to you in a web browser. Want to try it? (Requires opening a local URL)\"}}")
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	a := codex.New("ses_001", "run_001")
	_, ok, err := a.ParseTranscriptLine(ts, line)
	if err != nil {
		t.Fatalf("ParseTranscriptLine() error = %v", err)
	}
	if ok {
		t.Fatal("ParseTranscriptLine() ok = true, want false for browser-offer noise")
	}
}

func TestParseTranscriptLineAgentMessageQuestionWithContextKeepsFullText(t *testing.T) {
	line := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"item_6\",\"type\":\"agent_message\",\"text\":\"I checked the current workspace first and there isn’t a project here yet, just two image files on `/Users/canh/Desktop`. I need the site/app repo location before I can add a new SEO page. Also, if you want, I can use the visual companion for mockups and layout options; if not, I’ll keep this text-only.\\n\\nWhat is the project path or repo folder I should work in?\"}}")
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
	want := "I checked the current workspace first and there isn’t a project here yet, just two image files on `/Users/canh/Desktop`. I need the site/app repo location before I can add a new SEO page. Also, if you want, I can use the visual companion for mockups and layout options; if not, I’ll keep this text-only.\n\nWhat is the project path or repo folder I should work in?"
	if obs.Summary != want {
		t.Errorf("Summary = %q, want %q", obs.Summary, want)
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

	want := []string{"question", "blocked"}
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
