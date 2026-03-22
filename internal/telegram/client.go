package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       UpdateMessage  `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type UpdateMessage struct {
	MessageID int `json:"message_id"`
	UserID    int64
	ChatID    int64
	ThreadID  int
	Text      string `json:"text"`
}

type CallbackQuery struct {
	ID         string
	FromUserID int64
	Data       string
	Message    UpdateMessage
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

func NewClient(token, baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}

	return &Client{
		token:      token,
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL("/getUpdates"), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	q := req.URL.Query()
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	req.URL.RawQuery = q.Encode()

	var resp struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				MessageID       int    `json:"message_id"`
				MessageThreadID int    `json:"message_thread_id"`
				Text            string `json:"text"`
				From            struct {
					ID int64 `json:"id"`
				} `json:"from"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			} `json:"message"`
			CallbackQuery struct {
				ID   string `json:"id"`
				Data string `json:"data"`
				From struct {
					ID int64 `json:"id"`
				} `json:"from"`
				Message struct {
					MessageID       int    `json:"message_id"`
					MessageThreadID int    `json:"message_thread_id"`
					Text            string `json:"text"`
					Chat            struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"callback_query"`
		} `json:"result"`
	}

	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}

	updates := make([]Update, 0, len(resp.Result))
	for _, item := range resp.Result {
		updates = append(updates, Update{
			UpdateID: item.UpdateID,
			Message: UpdateMessage{
				MessageID: item.Message.MessageID,
				UserID:    item.Message.From.ID,
				ChatID:    item.Message.Chat.ID,
				ThreadID:  item.Message.MessageThreadID,
				Text:      item.Message.Text,
			},
		})
		if item.CallbackQuery.ID != "" {
			updates[len(updates)-1].CallbackQuery = &CallbackQuery{
				ID:         item.CallbackQuery.ID,
				FromUserID: item.CallbackQuery.From.ID,
				Data:       item.CallbackQuery.Data,
				Message: UpdateMessage{
					MessageID: item.CallbackQuery.Message.MessageID,
					ChatID:    item.CallbackQuery.Message.Chat.ID,
					ThreadID:  item.CallbackQuery.Message.MessageThreadID,
					Text:      item.CallbackQuery.Message.Text,
				},
			}
		}
	}

	return updates, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error {
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if threadID != nil {
		body["message_thread_id"] = *threadID
	}

	req, err := c.newJSONRequest(ctx, "/sendMessage", body)
	if err != nil {
		return err
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	return c.doJSON(req, &resp)
}

func (c *Client) SendMessageCard(ctx context.Context, chatID int64, threadID *int, text string, markup InlineKeyboardMarkup) (int, error) {
	body := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": markup,
	}
	if threadID != nil {
		body["message_thread_id"] = *threadID
	}

	req, err := c.newJSONRequest(ctx, "/sendMessage", body)
	if err != nil {
		return 0, err
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return 0, err
	}
	return resp.Result.MessageID, nil
}

func (c *Client) EditMessageCard(ctx context.Context, chatID int64, messageID int, text string, markup InlineKeyboardMarkup) error {
	req, err := c.newJSONRequest(ctx, "/editMessageText", map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"text":         text,
		"reply_markup": markup,
	})
	if err != nil {
		return err
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	return c.doJSON(req, &resp)
}

func (c *Client) AnswerCallback(ctx context.Context, callbackID, text string) error {
	body := map[string]any{
		"callback_query_id": callbackID,
	}
	if strings.TrimSpace(text) != "" {
		body["text"] = text
	}

	req, err := c.newJSONRequest(ctx, "/answerCallbackQuery", body)
	if err != nil {
		return err
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.doJSON(req, &resp)
}

func (c *Client) CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error) {
	req, err := c.newJSONRequest(ctx, "/createForumTopic", map[string]any{
		"chat_id": chatID,
		"name":    name,
	})
	if err != nil {
		return 0, err
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageThreadID int `json:"message_thread_id"`
		} `json:"result"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return 0, err
	}

	return resp.Result.MessageThreadID, nil
}

func (c *Client) DeleteForumTopic(ctx context.Context, chatID int64, threadID int) error {
	req, err := c.newJSONRequest(ctx, "/deleteForumTopic", map[string]any{
		"chat_id":           chatID,
		"message_thread_id": threadID,
	})
	if err != nil {
		return err
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result bool `json:"result"`
	}
	return c.doJSON(req, &resp)
}

func (c *Client) newJSONRequest(ctx context.Context, path string, body any) (*http.Request, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) doJSON(req *http.Request, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api error %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c *Client) apiURL(path string) string {
	return c.baseURL + "/bot" + c.token + path
}
