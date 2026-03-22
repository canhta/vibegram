package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetUpdatesParsesMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/getUpdates" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/getUpdates")
		}
		if got := r.URL.Query().Get("offset"); got != "10" {
			t.Fatalf("offset = %q, want %q", got, "10")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":11,"message":{"message_id":7,"from":{"id":1001},"chat":{"id":-100123},"message_thread_id":42,"text":"hello"}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	updates, err := client.GetUpdates(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("len(updates) = %d, want 1", len(updates))
	}
	if updates[0].UpdateID != 11 {
		t.Fatalf("UpdateID = %d, want 11", updates[0].UpdateID)
	}
	if updates[0].Message.ChatID != -100123 {
		t.Fatalf("ChatID = %d, want -100123", updates[0].Message.ChatID)
	}
	if updates[0].Message.ThreadID != 42 {
		t.Fatalf("ThreadID = %d, want 42", updates[0].Message.ThreadID)
	}
	if updates[0].Message.UserID != 1001 {
		t.Fatalf("UserID = %d, want 1001", updates[0].Message.UserID)
	}
	if updates[0].Message.Text != "hello" {
		t.Fatalf("Text = %q, want %q", updates[0].Message.Text, "hello")
	}
}

func TestClientSendMessageUsesThreadIDWhenProvided(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/sendMessage")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if body["chat_id"] != float64(-100123) {
			t.Fatalf("chat_id = %v, want -100123", body["chat_id"])
		}
		if body["message_thread_id"] != float64(42) {
			t.Fatalf("message_thread_id = %v, want 42", body["message_thread_id"])
		}
		if body["text"] != "hello" {
			t.Fatalf("text = %v, want hello", body["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	threadID := 42
	if err := client.SendMessage(context.Background(), -100123, &threadID, "hello"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

func TestClientCreateForumTopicReturnsThreadID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/createForumTopic" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/createForumTopic")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if body["chat_id"] != float64(-100123) {
			t.Fatalf("chat_id = %v, want -100123", body["chat_id"])
		}
		if body["name"] != "Session A" {
			t.Fatalf("name = %v, want Session A", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_thread_id":42}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	threadID, err := client.CreateForumTopic(context.Background(), -100123, "Session A")
	if err != nil {
		t.Fatalf("CreateForumTopic() error = %v", err)
	}
	if threadID != 42 {
		t.Fatalf("threadID = %d, want 42", threadID)
	}
}
