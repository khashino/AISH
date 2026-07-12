package provider

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
	TotalTokens  int  `json:"total_tokens"`
	Estimated    bool `json:"estimated"`
}

type StreamFunc func(string) error

type Client interface {
	Name() string
	Chat(context.Context, []Message) (string, error)
	Stream(context.Context, []Message, StreamFunc) (string, error)
}

type UsageReporter interface {
	LastUsage() Usage
}
