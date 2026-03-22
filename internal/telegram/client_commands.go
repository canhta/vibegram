package telegram

import (
	"context"
	"fmt"
)

func (c *Client) SetCommands(ctx context.Context, chatID int64, commands []BotCommand) error {
	req, err := c.newJSONRequest(ctx, "/setMyCommands", map[string]any{
		"commands": commands,
		"scope": map[string]any{
			"type":    "chat",
			"chat_id": chatID,
		},
	})
	if err != nil {
		return err
	}

	var response struct {
		OK     bool   `json:"ok"`
		Result bool   `json:"result"`
		Error  string `json:"description"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return err
	}
	if !response.OK || !response.Result {
		return fmt.Errorf("telegram setMyCommands failed")
	}
	return nil
}
