package config

import "os"

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func LoadConfig() *Config {
	baseURL := os.Getenv("KAO_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("KAO_MODEL")
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	return &Config{
		BaseURL: baseURL,
		APIKey:  os.Getenv("KAO_API_KEY"),
		Model:   model,
	}
}
