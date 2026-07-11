package openai

import "github.com/khashino/AISH/internal/provider/openaicompat"

func New(baseURL, model, apiKey string) *openaicompat.Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return openaicompat.New("openai", baseURL, model, apiKey)
}
