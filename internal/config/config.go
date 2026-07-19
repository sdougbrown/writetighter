package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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
	Provider     string `toml:"provider"`
	BaseURL      string `toml:"base_url"`
	Model        string `toml:"model"`
	APIKeyEnv    string `toml:"api_key_env"`
	Timeout      string `toml:"timeout"`
	ResponseMode string `toml:"response_mode"`
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

func MergeConfigs(project *ProjectConfig, user *UserConfig) (*MergedConfig, error) {
	return &MergedConfig{Project: project, User: user}, nil
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
	return &cfg, nil
}

func normalizeTerm(term string) string {
	return strings.ToLower(strings.TrimSpace(term))
}

func configError(msg string) error { return fmt.Errorf("config error: %s", msg) }
