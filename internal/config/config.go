package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultMaxOutputTokens  = 2048
	BudgetSafetyTokens      = 512
	MinEditableSourceTokens = 512
)

type ProjectConfig struct {
	Profile ProfileConfig `toml:"profile"`
	Terms   []TermEntry   `toml:"terms"`
}

type ProfileConfig struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
}

type TermEntry struct {
	Term          string   `toml:"term"`
	PartsOfSpeech []string `toml:"parts_of_speech"`
	Definition    string   `toml:"definition"`
	Override      bool     `toml:"override"`
	Reason        string   `toml:"reason"`
}

type UserConfig struct {
	Profile ProfileConfig `toml:"profile"`
	LLM     LLMConfig     `toml:"llm"`
}

type LLMConfig struct {
	Provider            string `toml:"provider"`
	BaseURL             string `toml:"base_url"`
	Model               string `toml:"model"`
	APIKey              string `toml:"api_key,omitempty"`
	APIKeyEnv           string `toml:"api_key_env,omitempty"`
	Timeout             string `toml:"timeout"`
	ResponseMode        string `toml:"response_mode"`
	MaxRequests         int    `toml:"max_requests,omitempty"`
	ContextWindowTokens int    `toml:"context_window_tokens,omitzero"`
	MaxOutputTokens     int    `toml:"max_output_tokens,omitzero"`
	ContextWindowModel  string `toml:"context_window_model,omitempty"`
}

type MergedConfig struct {
	Project *ProjectConfig
	User    *UserConfig
}

func LoadProjectConfig(path string) (*ProjectConfig, error) {
	var cfg ProjectConfig
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("config %s: unknown key(s): %v", path, undecoded)
	}
	if err := ValidateTermBase(cfg.Terms); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func DiscoverProjectConfig() (*ProjectConfig, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	for {
		path := filepath.Join(dir, ".writetighter.toml")
		if _, err := os.Stat(path); err == nil {
			cfg, err := LoadProjectConfig(path)
			return cfg, path, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", os.ErrNotExist
		}
		dir = parent
	}
}

// UserConfigPath returns the platform-specific path used for new user config.
func UserConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "writetighter", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "writetighter", "config.toml"), nil
}

func LoadUserConfig() (*UserConfig, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path := filepath.Join(xdg, "writetighter", "config.toml")
		if cfg, err := loadUserConfigFile(path); err == nil {
			return cfg, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return loadUserConfigFile(filepath.Join(home, ".config", "writetighter", "config.toml"))
}

// WriteUserConfig atomically writes a user config with private permissions.
func WriteUserConfig(cfg *UserConfig) (string, error) {
	if cfg == nil {
		return "", errors.New("user config is required")
	}
	path, err := UserConfigPath()
	if err != nil {
		return "", err
	}
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Encode(cfg); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data.Bytes()); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if _, err := loadUserConfigFile(tmpPath); err != nil {
		return "", fmt.Errorf("validate generated config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func MergeConfigs(project *ProjectConfig, user *UserConfig) (*MergedConfig, error) {
	return &MergedConfig{Project: project, User: user}, nil
}

// SanitizedTOML returns the user configuration in TOML format.
// It redacts the API key but preserves the api_key_env field, which contains
// the name of the environment variable rather than the secret itself.
// The shallow copy is safe because UserConfig and LLMConfig contain only value
// types; if either gains a slice or pointer field, copy the fields explicitly.
func (c *UserConfig) SanitizedTOML() (string, error) {
	sanitized := *c
	sanitized.LLM.APIKey = ""
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(&sanitized); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func ValidateTermBase(entries []TermEntry) error {
	return validateTermBase(entries)
}

func loadUserConfigFile(path string) (*UserConfig, error) {
	var cfg UserConfig
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("config %s: unknown key(s): %v", path, undecoded)
	}
	if cfg.LLM.APIKey != "" && cfg.LLM.APIKeyEnv != "" {
		return nil, fmt.Errorf("config %s: llm.api_key and llm.api_key_env are mutually exclusive", path)
	}
	if md.IsDefined("llm", "context_window_tokens") && cfg.LLM.ContextWindowTokens <= 0 {
		return nil, fmt.Errorf("config %s: llm.context_window_tokens must be > 0", path)
	}
	if md.IsDefined("llm", "max_output_tokens") && cfg.LLM.MaxOutputTokens <= 0 {
		return nil, fmt.Errorf("config %s: llm.max_output_tokens must be > 0", path)
	}
	if cfg.LLM.ContextWindowTokens > 0 && cfg.LLM.MaxOutputTokens > 0 && cfg.LLM.MaxOutputTokens >= cfg.LLM.ContextWindowTokens {
		return nil, fmt.Errorf("config %s: llm.max_output_tokens (%d) must be less than llm.context_window_tokens (%d)", path, cfg.LLM.MaxOutputTokens, cfg.LLM.ContextWindowTokens)
	}
	if md.IsDefined("llm", "context_window_model") && cfg.LLM.ContextWindowModel == "" {
		return nil, fmt.Errorf("config %s: llm.context_window_model must be non-empty if set", path)
	}
	return &cfg, nil
}

func normalizeTerm(term string) string {
	return strings.ToLower(strings.TrimSpace(term))
}
