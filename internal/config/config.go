package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	ListenAddr          string         `json:"listen_addr"`
	DataPath            string         `json:"data_path"`
	PingIntervalSeconds int            `json:"ping_interval_seconds"`
	PingCount           int            `json:"ping_count"`
	PingTimeoutSeconds  int            `json:"ping_timeout_seconds"`
	Targets             []TargetConfig `json:"targets"`
}

type TargetConfig struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Target string `json:"target"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Default() Config {
	return Config{
		ListenAddr:          "0.0.0.0:8080",
		DataPath:            "/var/lib/netwatch/samples.jsonl",
		PingIntervalSeconds: 30,
		PingCount:           10,
		PingTimeoutSeconds:  15,
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return errors.New("listen_addr is required")
	}
	if strings.TrimSpace(c.DataPath) == "" {
		return errors.New("data_path is required")
	}
	if c.PingIntervalSeconds <= 0 {
		return errors.New("ping_interval_seconds must be greater than 0")
	}
	if c.PingCount <= 0 {
		return errors.New("ping_count must be greater than 0")
	}
	if c.PingTimeoutSeconds <= 0 {
		return errors.New("ping_timeout_seconds must be greater than 0")
	}
	if len(c.Targets) == 0 {
		return errors.New("targets must not be empty")
	}

	names := make(map[string]struct{}, len(c.Targets))
	for i, target := range c.Targets {
		if strings.TrimSpace(target.Name) == "" {
			return fmt.Errorf("targets[%d].name is required", i)
		}
		if _, ok := names[target.Name]; ok {
			return fmt.Errorf("duplicate target name: %s", target.Name)
		}
		names[target.Name] = struct{}{}

		if target.Type != "ping" {
			return fmt.Errorf("targets[%d].type must be ping in Phase 1", i)
		}
		if strings.TrimSpace(target.Target) == "" {
			return fmt.Errorf("targets[%d].target is required", i)
		}
	}

	return nil
}
