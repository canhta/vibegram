package roles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OpenAICaller struct {
	APIKey string
	Model  string
}

func NewOpenAICaller(apiKey, model string) *OpenAICaller {
	return &OpenAICaller{APIKey: apiKey, Model: model}
}

func (c *OpenAICaller) Call(ctx context.Context, prompt string) (string, error) {
	body := map[string]any{
		"model": c.Model,
		"input": prompt,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai api error %d: %s", resp.StatusCode, string(respData))
	}

	var result struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Output) == 0 || len(result.Output[0].Content) == 0 {
		return "", fmt.Errorf("empty response from openai")
	}

	return result.Output[0].Content[0].Text, nil
}
