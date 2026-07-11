package ollama

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
	BaseURL, Model string
	HTTP           *http.Client
}

func New(baseURL, model string) *Client {
	return &Client{strings.TrimRight(baseURL, "/"), model, &http.Client{Timeout: 10 * time.Minute}}
}
func (c *Client) Name() string { return "ollama" }
func (c *Client) request(ctx context.Context, messages []provider.Message, stream bool) (*http.Response, error) {
	payload, err := json.Marshal(map[string]any{"model": c.Model, "messages": messages, "stream": stream})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", c.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("Ollama returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}
func (c *Client) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	resp, err := c.request(ctx, messages, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Message provider.Message `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
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
		var part struct {
			Message provider.Message `json:"message"`
			Done    bool             `json:"done"`
		}
		if json.Unmarshal(s.Bytes(), &part) != nil {
			continue
		}
		if part.Message.Content != "" {
			full.WriteString(part.Message.Content)
			if err := emit(part.Message.Content); err != nil {
				return full.String(), err
			}
		}
		if part.Done {
			break
		}
	}
	return full.String(), s.Err()
}
