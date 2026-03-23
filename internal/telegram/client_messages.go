package telegram

import "context"

func (c *Client) SendMessage(ctx context.Context, chatID int64, threadID *int, text string) error {
	body := map[string]any{
		"chat_id": chatID,
		"text":    ClampMessageText(text),
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
		"text":         ClampMessageText(text),
		"reply_markup": normalizeReplyMarkup(markup),
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
		"text":         ClampMessageText(text),
		"reply_markup": normalizeReplyMarkup(markup),
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
	if text != "" {
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

func (c *Client) EditForumTopic(ctx context.Context, chatID int64, threadID int, name string) error {
	req, err := c.newJSONRequest(ctx, "/editForumTopic", map[string]any{
		"chat_id":           chatID,
		"message_thread_id": threadID,
		"name":              name,
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

func normalizeReplyMarkup(markup InlineKeyboardMarkup) InlineKeyboardMarkup {
	if markup.InlineKeyboard == nil {
		markup.InlineKeyboard = [][]InlineKeyboardButton{}
	}
	return markup
}
