package telegram

import (
	"context"
	"fmt"
	"net/http"
)

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
