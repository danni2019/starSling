package configstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danni2019/starSling/internal/config"
)

const (
	defaultConfigName = "default"
	indexFileName     = "index.json"
)

type Index struct {
	Default string `json:"default"`
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err == nil && base != "" {
		return filepath.Join(base, "starsling", "configs"), nil
	}
	return filepath.Join("runtime", "configs"), nil
}

func DefaultName() string {
	return defaultConfigName
}

func NormalizeName(name string) (string, error) {
	return sanitizeName(name)
}

func Ensure() (Index, error) {
	dir, err := Dir()
	if err != nil {
		return Index{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Index{}, fmt.Errorf("create config dir: %w", err)
	}

	names, err := listNames(dir)
	if err != nil {
		return Index{}, err
	}

	if len(names) == 0 {
		cfg, err := config.Default()
		if err != nil {
			return Index{}, err
		}
		if err := Save(defaultConfigName, cfg); err != nil {
			return Index{}, err
		}
		names = []string{defaultConfigName}
	}

	idx, err := loadIndex(dir)
	if err != nil {
		return Index{}, err
	}

	if idx.Default == "" || !contains(names, idx.Default) {
		idx.Default = names[0]
		if err := saveIndex(dir, idx); err != nil {
			return Index{}, err
		}
	}

	return idx, nil
}

func List() ([]string, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if _, err := Ensure(); err != nil {
		return nil, err
	}
	names, err := listNames(dir)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func Exists(name string) (bool, error) {
	path, err := pathFor(name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func Load(name string) (config.Config, error) {
	path, err := pathFor(name)
	if err != nil {
		return config.Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config.Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func LoadDefault() (string, config.Config, error) {
	idx, err := Ensure()
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, err := Load(idx.Default)
	if err != nil {
		return "", config.Config{}, err
	}
	return idx.Default, cfg, nil
}

func Save(name string, cfg config.Config) error {
	path, err := pathFor(name)
	if err != nil {
		return err
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func Delete(name string) error {
	path, err := pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete config: %w", err)
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	names, err := listNames(dir)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no configs remain after delete")
	}
	idx, err := loadIndex(dir)
	if err != nil {
		return err
	}
	if idx.Default == "" {
		idx.Default = names[0]
		return saveIndex(dir, idx)
	}
	if idx.Default == name {
		idx.Default = names[0]
		return saveIndex(dir, idx)
	}
	return nil
}

func SetDefault(name string) error {
	if _, err := Ensure(); err != nil {
		return err
	}
	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("config not found: %s", name)
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	idx, err := loadIndex(dir)
	if err != nil {
		return err
	}
	idx.Default = name
	return saveIndex(dir, idx)
}

func saveIndexFromDefault(idx Index) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return saveIndex(dir, idx)
}

func loadIndex(dir string) (Index, error) {
	path := filepath.Join(dir, indexFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Index{}, nil
		}
		return Index{}, fmt.Errorf("read config index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("parse config index: %w", err)
	}
	return idx, nil
}

func saveIndex(dir string, idx Index) error {
	if idx.Default == "" {
		return fmt.Errorf("default config is required")
	}
	payload, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config index: %w", err)
	}
	path := filepath.Join(dir, indexFileName)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write config index: %w", err)
	}
	return nil
}

func listNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == indexFileName || !strings.HasSuffix(name, ".json") {
			continue
		}
		trimmed := strings.TrimSuffix(name, ".json")
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	sort.Strings(names)
	return names, nil
}

func pathFor(name string) (string, error) {
	clean, err := sanitizeName(name)
	if err != nil {
		return "", err
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, clean+".json"), nil
}

func sanitizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(name, ".json"))
	if trimmed == "" {
		return "", fmt.Errorf("config name is required")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("config name must not include path separators")
	}
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' {
			continue
		}
		return "", fmt.Errorf("config name supports letters, numbers, '-' and '_' only")
	}
	return trimmed, nil
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
