package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	BaseURL              string  `json:"base_url"`
	Model                string  `json:"model"`
	APIKeyEnv            string  `json:"api_key_env"`
	InputCostPerMillion  float64 `json:"input_cost_per_million,omitempty"`
	OutputCostPerMillion float64 `json:"output_cost_per_million,omitempty"`
}
type DocumentsConfig struct {
	Folder            string `json:"folder"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	TopK              int    `json:"top_k"`
}
type Config struct {
	ActiveProvider string                    `json:"active_provider"`
	Providers      map[string]ProviderConfig `json:"providers"`
	RequireConfirm bool                      `json:"require_confirmation"`
	Documents      DocumentsConfig           `json:"documents"`
	ShowUsage      string                    `json:"show_usage"`
}

func Default() Config {
	return Config{ActiveProvider: "ollama", Providers: map[string]ProviderConfig{
		"ollama":   {BaseURL: "http://localhost:11434", Model: "llama3.2"},
		"llamacpp": {BaseURL: "http://localhost:8080/v1", Model: "local-model"},
		"openai":   {BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini", APIKeyEnv: "OPENAI_API_KEY"},
		"claude":   {BaseURL: "https://api.anthropic.com/v1", Model: "claude-sonnet-4-5", APIKeyEnv: "ANTHROPIC_API_KEY"},
		"gemini":   {BaseURL: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-2.5-flash", APIKeyEnv: "GEMINI_API_KEY"},
	}, RequireConfirm: true, ShowUsage: "summary", Documents: DocumentsConfig{Folder: "documents", EmbeddingProvider: "ollama", EmbeddingModel: "nomic-embed-text", TopK: 5}}
}
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "aish"), nil
}
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := Save(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}
