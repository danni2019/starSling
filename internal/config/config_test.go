package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected default config: %v", err)
	}
	if cfg.Runtime.CheckIntervalSeconds <= 0 {
		t.Fatalf("expected runtime check interval")
	}
	if cfg.LiveMD.Port <= 0 {
		t.Fatalf("expected live-md port")
	}
}

func TestLoadOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "override.json")
	override := []byte(`{"live-md": {"host": "127.0.0.1", "port": 4001}}`)

	if err := os.WriteFile(path, override, 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load override: %v", err)
	}
	if cfg.LiveMD.Host != "127.0.0.1" || cfg.LiveMD.Port != 4001 {
		t.Fatalf("override not applied")
	}
	if cfg.Runtime.CheckIntervalSeconds <= 0 {
		t.Fatalf("expected runtime defaults to remain")
	}
}
