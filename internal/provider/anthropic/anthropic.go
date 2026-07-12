package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/khashino/AISH/internal/provider"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Model, APIKey string
	HTTP                   *http.Client
	last                   provider.Usage
}

func New(baseURL, model, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}
func (c *Client) Name() string              { return "claude" }
func (c *Client) LastUsage() provider.Usage { return c.last }
func (c *Client) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	var system string
	filtered := make([]provider.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			if system != "" {
				system += "\n"
			}
			system += m.Content
		} else {
			filtered = append(filtered, m)
		}
	}
	body := map[string]any{"model": c.Model, "max_tokens": 4096, "messages": filtered}
	if system != "" {
		body["system"] = system
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach Claude: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("Claude returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out struct {
		Content []struct{ Type, Text string } `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, x := range out.Content {
		if x.Type == "text" {
			b.WriteString(x.Text)
		}
	}
	c.last = provider.Usage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens, TotalTokens: out.Usage.InputTokens + out.Usage.OutputTokens}
	return b.String(), nil
}
func (c *Client) Stream(ctx context.Context, m []provider.Message, emit provider.StreamFunc) (string, error) {
	text, err := c.Chat(ctx, m)
	if err == nil {
		err = emit(text)
	}
	return text, err
}
