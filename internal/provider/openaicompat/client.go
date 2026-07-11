package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/khashino/AISH/internal/provider"
)

type Client struct {
	ProviderName string
	BaseURL      string
	Model        string
	APIKey       string
	HTTP         *http.Client
}

func New(name, baseURL, model, apiKey string) *Client {
	return &Client{ProviderName: name, BaseURL: strings.TrimRight(baseURL, "/"), Model: model, APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}
func (c *Client) Name() string { return c.ProviderName }

func (c *Client) request(ctx context.Context, messages []provider.Message, stream bool) (*http.Response, error) {
	body := map[string]any{"model": c.Model, "messages": messages, "stream": stream}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s at %s: %w", c.ProviderName, c.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("%s returned %s: %s", c.ProviderName, resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (c *Client) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	resp, err := c.request(ctx, messages, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Choices []struct {
			Message provider.Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("%s returned no choices", c.ProviderName)
	}
	return result.Choices[0].Message.Content, nil
}

func (c *Client) Stream(ctx context.Context, messages []provider.Message, emit provider.StreamFunc) (string, error) {
	resp, err := c.request(ctx, messages, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var full strings.Builder
	s := bufio.NewScanner(resp.Body)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		text := chunk.Choices[0].Delta.Content
		if text != "" {
			full.WriteString(text)
			if err := emit(text); err != nil {
				return full.String(), err
			}
		}
	}
	return full.String(), s.Err()
}
