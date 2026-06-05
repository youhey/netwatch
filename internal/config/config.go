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
	DNSIntervalSeconds  int            `json:"dns_interval_seconds"`
	DNSTimeoutSeconds   int            `json:"dns_timeout_seconds"`
	HTTPIntervalSeconds int            `json:"http_interval_seconds"`
	HTTPTimeoutSeconds  int            `json:"http_timeout_seconds"`
	Targets             []TargetConfig `json:"targets"`
}

type TargetConfig struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	Hostname string `json:"hostname"`
	URL      string `json:"url"`
	Method   string `json:"method"`
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
		DNSIntervalSeconds:  60,
		DNSTimeoutSeconds:   5,
		HTTPIntervalSeconds: 60,
		HTTPTimeoutSeconds:  10,
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
	if c.DNSIntervalSeconds <= 0 {
		return errors.New("dns_interval_seconds must be greater than 0")
	}
	if c.DNSTimeoutSeconds <= 0 {
		return errors.New("dns_timeout_seconds must be greater than 0")
	}
	if c.HTTPIntervalSeconds <= 0 {
		return errors.New("http_interval_seconds must be greater than 0")
	}
	if c.HTTPTimeoutSeconds <= 0 {
		return errors.New("http_timeout_seconds must be greater than 0")
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

		switch target.Type {
		case "ping":
			if strings.TrimSpace(target.Target) == "" {
				return fmt.Errorf("targets[%d].target is required for ping target", i)
			}
		case "dns":
			if strings.TrimSpace(target.Hostname) == "" {
				return fmt.Errorf("targets[%d].hostname is required for dns target", i)
			}
		case "http":
			if strings.TrimSpace(target.URL) == "" {
				return fmt.Errorf("targets[%d].url is required for http target", i)
			}
			if target.Method != "" && strings.ToUpper(target.Method) != "GET" {
				return fmt.Errorf("targets[%d].method must be GET in Phase 2", i)
			}
		default:
			return fmt.Errorf("targets[%d].type must be one of ping, dns, http", i)
		}
	}

	return nil
}
