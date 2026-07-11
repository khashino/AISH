package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/khashino/AISH/internal/provider"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Model, APIKey string
	HTTP                   *http.Client
}

func New(baseURL, model, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &Client{strings.TrimRight(baseURL, "/"), model, apiKey, &http.Client{Timeout: 10 * time.Minute}}
}
func (c *Client) Name() string { return "gemini" }
func (c *Client) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	contents := make([]map[string]any, 0, len(messages))
	var system string
	for _, m := range messages {
		if m.Role == "system" {
			system += m.Content + "\n"
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{"role": role, "parts": []map[string]string{{"text": m.Content}}})
	}
	body := map[string]any{"contents": contents}
	if strings.TrimSpace(system) != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": strings.TrimSpace(system)}}}
	}
	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.BaseURL, url.PathEscape(c.Model), url.QueryEscape(c.APIKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach Gemini: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("Gemini returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("Gemini returned no candidates")
	}
	var b strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		b.WriteString(p.Text)
	}
	return b.String(), nil
}
func (c *Client) Stream(ctx context.Context, m []provider.Message, emit provider.StreamFunc) (string, error) {
	text, err := c.Chat(ctx, m)
	if err == nil {
		err = emit(text)
	}
	return text, err
}
