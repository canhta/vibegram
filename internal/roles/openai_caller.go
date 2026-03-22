package roles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAICaller struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewOpenAICaller(apiKey, model, baseURL string) *OpenAICaller {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAICaller{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: strings.TrimRight(baseURL, "/"),
	}
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(data))
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
		OutputText string `json:"output_text"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if strings.TrimSpace(result.OutputText) != "" {
		return strings.TrimSpace(result.OutputText), nil
	}

	for _, item := range result.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), nil
			}
		}
	}

	return "", fmt.Errorf("empty response from openai")
}
