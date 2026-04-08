package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type RuntimeConfig struct {
	CheckIntervalSeconds   int `json:"check_interval_seconds"`
	IdleLogIntervalSeconds int `json:"idle_log_interval_seconds"`
}

type LiveMDConfig struct {
	API         string   `json:"api"`
	Protocol    string   `json:"protocol"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Password    string   `json:"pwd"`
	Instruments []string `json:"instruments"`
}

type Config struct {
	Runtime RuntimeConfig `json:"runtime"`
	LiveMD  LiveMDConfig  `json:"live-md"`
}

//go:embed defaults.json
var defaultConfig []byte

func Default() (Config, error) {
	var cfg Config
	if err := json.Unmarshal(defaultConfig, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse embedded config: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Load(path string) (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Normalize() {
	c.LiveMD.API = strings.ToLower(strings.TrimSpace(c.LiveMD.API))
	c.LiveMD.Protocol = strings.ToLower(strings.TrimSpace(c.LiveMD.Protocol))
	c.LiveMD.Host = strings.TrimSpace(c.LiveMD.Host)
	c.LiveMD.Username = strings.TrimSpace(c.LiveMD.Username)
	if c.LiveMD.API == "" {
		c.LiveMD.API = "ctp"
	}
	if c.LiveMD.Protocol == "" {
		c.LiveMD.Protocol = "tcp"
	}
	if len(c.LiveMD.Instruments) > 0 {
		c.LiveMD.Instruments = normalizeList(c.LiveMD.Instruments)
	}
}

func normalizeList(items []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (c Config) Validate() error {
	if c.Runtime.CheckIntervalSeconds <= 0 {
		return fmt.Errorf("runtime.check_interval_seconds must be > 0")
	}
	if c.Runtime.IdleLogIntervalSeconds <= 0 {
		return fmt.Errorf("runtime.idle_log_interval_seconds must be > 0")
	}
	if c.LiveMD.Port < 0 || c.LiveMD.Port > 65535 {
		return fmt.Errorf("live-md.port must be 0 (unset) or between 1 and 65535")
	}
	return nil
}

func (c Config) LiveMDConfigured() bool {
	host := strings.TrimSpace(c.LiveMD.Host)
	return host != "" && c.LiveMD.Port > 0 && c.LiveMD.Port <= 65535
}

func (c Config) ValidateLiveMD() error {
	host := strings.TrimSpace(c.LiveMD.Host)
	if host == "" {
		return fmt.Errorf("live-md.host must be configured before starting live market data")
	}
	if c.LiveMD.Port <= 0 || c.LiveMD.Port > 65535 {
		if c.LiveMD.Port == 0 {
			return fmt.Errorf("live-md.port must be configured before starting live market data")
		}
		return fmt.Errorf("live-md.port must be between 1 and 65535")
	}
	if c.LiveMD.Protocol == "" {
		return fmt.Errorf("live-md.protocol is required")
	}
	if c.LiveMD.API == "" {
		return fmt.Errorf("live-md.api is required")
	}
	return nil
}
