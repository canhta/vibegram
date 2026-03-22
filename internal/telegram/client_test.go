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

func TestClientGetUpdatesParsesCallbackQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":12,"callback_query":{"id":"cb-1","data":"set:launch","from":{"id":1001},"message":{"message_id":9,"chat":{"id":-100123},"message_thread_id":42,"text":"Setup card"}}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	updates, err := client.GetUpdates(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("len(updates) = %d, want 1", len(updates))
	}
	if updates[0].CallbackQuery == nil {
		t.Fatal("CallbackQuery = nil, want parsed callback")
	}
	if updates[0].CallbackQuery.ID != "cb-1" {
		t.Fatalf("CallbackQuery.ID = %q, want %q", updates[0].CallbackQuery.ID, "cb-1")
	}
	if updates[0].CallbackQuery.Data != "set:launch" {
		t.Fatalf("CallbackQuery.Data = %q, want %q", updates[0].CallbackQuery.Data, "set:launch")
	}
	if updates[0].CallbackQuery.FromUserID != 1001 {
		t.Fatalf("CallbackQuery.FromUserID = %d, want 1001", updates[0].CallbackQuery.FromUserID)
	}
	if updates[0].CallbackQuery.Message.ThreadID != 42 {
		t.Fatalf("CallbackQuery.Message.ThreadID = %d, want 42", updates[0].CallbackQuery.Message.ThreadID)
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

func TestClientSendMessageCardReturnsMessageID(t *testing.T) {
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
		if body["text"] != "Setup card" {
			t.Fatalf("text = %v, want Setup card", body["text"])
		}
		replyMarkup, ok := body["reply_markup"].(map[string]any)
		if !ok {
			t.Fatalf("reply_markup = %T, want object", body["reply_markup"])
		}
		rows, ok := replyMarkup["inline_keyboard"].([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("inline_keyboard = %v, want one row", replyMarkup["inline_keyboard"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	threadID := 42
	messageID, err := client.SendMessageCard(context.Background(), -100123, &threadID, "Setup card", InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{{
			{Text: "Launch", CallbackData: "set:launch"},
		}},
	})
	if err != nil {
		t.Fatalf("SendMessageCard() error = %v", err)
	}
	if messageID != 77 {
		t.Fatalf("messageID = %d, want 77", messageID)
	}
}

func TestClientEditMessageCardUsesReplyMarkup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/editMessageText" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/editMessageText")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["message_id"] != float64(77) {
			t.Fatalf("message_id = %v, want 77", body["message_id"])
		}
		if body["text"] != "Launching..." {
			t.Fatalf("text = %v, want Launching...", body["text"])
		}
		replyMarkup, ok := body["reply_markup"].(map[string]any)
		if !ok {
			t.Fatalf("reply_markup = %T, want object", body["reply_markup"])
		}
		if _, ok := replyMarkup["inline_keyboard"]; !ok {
			t.Fatalf("reply_markup = %v, want inline_keyboard", replyMarkup)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	err := client.EditMessageCard(context.Background(), -100123, 77, "Launching...", InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{},
	})
	if err != nil {
		t.Fatalf("EditMessageCard() error = %v", err)
	}
}

func TestClientEditMessageCardZeroMarkupUsesEmptyInlineKeyboardArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		replyMarkup, ok := body["reply_markup"].(map[string]any)
		if !ok {
			t.Fatalf("reply_markup = %T, want object", body["reply_markup"])
		}
		rows, ok := replyMarkup["inline_keyboard"].([]any)
		if !ok {
			t.Fatalf("inline_keyboard = %T, want array", replyMarkup["inline_keyboard"])
		}
		if len(rows) != 0 {
			t.Fatalf("inline_keyboard len = %d, want 0", len(rows))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	if err := client.EditMessageCard(context.Background(), -100123, 77, "Launching...", InlineKeyboardMarkup{}); err != nil {
		t.Fatalf("EditMessageCard() error = %v", err)
	}
}

func TestClientAnswerCallbackSendsCallbackIDAndText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/answerCallbackQuery" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/answerCallbackQuery")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["callback_query_id"] != "cb-1" {
			t.Fatalf("callback_query_id = %v, want cb-1", body["callback_query_id"])
		}
		if body["text"] != "Launching" {
			t.Fatalf("text = %v, want Launching", body["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	if err := client.AnswerCallback(context.Background(), "cb-1", "Launching"); err != nil {
		t.Fatalf("AnswerCallback() error = %v", err)
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

func TestClientDeleteForumTopicUsesThreadID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/deleteForumTopic" {
			t.Fatalf("Path = %q, want %q", r.URL.Path, "/bottest-token/deleteForumTopic")
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

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	if err := client.DeleteForumTopic(context.Background(), -100123, 42); err != nil {
		t.Fatalf("DeleteForumTopic() error = %v", err)
	}
}
