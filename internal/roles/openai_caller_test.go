package roles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICallerUsesConfiguredBaseURL(t *testing.T) {
	var (
		gotPath   string
		gotAuth   string
		gotModel  string
		gotInput  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		gotModel, _ = body["model"].(string)
		gotInput, _ = body["input"].(string)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"content":[{"text":"{\"action\":\"noop\"}"}]}]}`))
	}))
	defer server.Close()

	caller := NewOpenAICaller("test-key", "gpt-5.4", server.URL+"/v1")
	out, err := caller.Call(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if gotPath != "/v1/responses" {
		t.Fatalf("Path = %q, want %q", gotPath, "/v1/responses")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotModel != "gpt-5.4" {
		t.Fatalf("model = %q, want %q", gotModel, "gpt-5.4")
	}
	if gotInput != "test prompt" {
		t.Fatalf("input = %q, want %q", gotInput, "test prompt")
	}
	if out != "{\"action\":\"noop\"}" {
		t.Fatalf("output = %q, want %q", out, "{\"action\":\"noop\"}")
	}
}

func TestOpenAICallerSkipsReasoningItemsAndReturnsFinalMessageText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": [
				{"type":"reasoning","summary":[]},
				{"type":"message","content":[{"type":"output_text","text":"validated brief"}]}
			]
		}`))
	}))
	defer server.Close()

	caller := NewOpenAICaller("test-key", "gpt-5.4", server.URL+"/v1")
	out, err := caller.Call(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if out != "validated brief" {
		t.Fatalf("output = %q, want %q", out, "validated brief")
	}
}
