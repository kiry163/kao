package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider      string `yaml:"provider"`
	APIKey        string `yaml:"api_key"`
	Model         string `yaml:"model"`
	BaseURL       string `yaml:"base_url"`
	Thinking      bool   `yaml:"thinking"`
	SnapshotLines int    `yaml:"snapshot_lines"`
}

// DefaultPath returns the config file path ~/.config/kao/config.yaml.
// os.UserConfigDir() is deliberately not used: on macOS it resolves to
// ~/Library/Application Support.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "~"
	}
	return filepath.Join(home, ".config", "kao", "config.yaml")
}

var allowedProviders = []string{"openai", "openai_compatible", "qwen", "deepseek"}

// DefaultConfigYAML returns the default configuration written on first run.
// provider is keyless openai_compatible so the file loads without an API key;
// model and base_url are sensible placeholders the user edits.
func DefaultConfigYAML() string {
	return `provider: openai_compatible
api_key: ""
model: gpt-4o-mini
base_url: http://127.0.0.1:11434/v1
thinking: false
snapshot_lines: 300
`
}

// EnsureDefault creates the config file at path with default content if it
// does not already exist. It returns the path and whether the file was
// created. Creating the parent dirs is included.
func EnsureDefault(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(DefaultConfigYAML()), 0o644); err != nil {
		return false, fmt.Errorf("创建配置文件失败: %w", err)
	}
	return true, nil
}

// Load reads and validates the YAML config at path. Load returns an error
// describing the problem; it never falls back to defaults for absent keys other
// than thinking (false) and snapshot_lines (300).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	cfg.Provider = strings.TrimSpace(cfg.Provider)
	if !contains(allowedProviders, cfg.Provider) {
		return nil, fmt.Errorf("provider 必须是 %s 之一 (当前: %q)", strings.Join(allowedProviders, " | "), cfg.Provider)
	}
	if cfg.Provider != "openai_compatible" && strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("provider %q 需要 api_key", cfg.Provider)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	if cfg.Provider == "openai_compatible" && strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("provider %q 需要 base_url", cfg.Provider)
	}
	if cfg.SnapshotLines < 0 {
		return nil, fmt.Errorf("snapshot_lines 不能为负数 (当前: %d)", cfg.SnapshotLines)
	}
	if cfg.SnapshotLines == 0 {
		cfg.SnapshotLines = 300
	}
	return &cfg, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
