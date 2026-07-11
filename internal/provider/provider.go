package provider

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamFunc func(string) error

type Client interface {
	Name() string
	Chat(context.Context, []Message) (string, error)
	Stream(context.Context, []Message, StreamFunc) (string, error)
}
