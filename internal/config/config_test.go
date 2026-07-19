package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTermBase(t *testing.T) {
	if err := ValidateTermBase([]TermEntry{{Term: ""}}); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateTermBase([]TermEntry{{Term: "A"}, {Term: "a"}}); err == nil {
		t.Fatal("expected duplicate error")
	}
	if err := ValidateTermBase([]TermEntry{{Term: "A", Override: true}}); err == nil {
		t.Fatal("expected override reason error")
	}
}

func TestLoadAndDiscoverProjectConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".writetighter.toml")
	data := []byte("[profile]\nid='core'\nversion='1'\n[[terms]]\nterm='alpha'\ndefinition='x'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile.ID != "core" {
		t.Fatal("bad profile id")
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_, discovered, err := DiscoverProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	if discovered != path {
		t.Fatalf("got %s", discovered)
	}
}

func TestLoadUserConfigXDG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "writetighter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[llm]\nprovider='x'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Provider != "x" {
		t.Fatal("bad provider")
	}
}
