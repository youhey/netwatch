package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	ListenAddr           string         `json:"listen_addr"`
	DataPath             string         `json:"data_path"`
	DataDir              string         `json:"data_dir"`
	DataFilePattern      string         `json:"data_file_pattern"`
	RetentionDays        int            `json:"retention_days"`
	PingIntervalSeconds  int            `json:"ping_interval_seconds"`
	PingCount            int            `json:"ping_count"`
	PingTimeoutSeconds   int            `json:"ping_timeout_seconds"`
	DNSIntervalSeconds   int            `json:"dns_interval_seconds"`
	DNSTimeoutSeconds    int            `json:"dns_timeout_seconds"`
	HTTPIntervalSeconds  int            `json:"http_interval_seconds"`
	HTTPTimeoutSeconds   int            `json:"http_timeout_seconds"`
	HTTPDisableKeepAlive bool           `json:"http_disable_keepalive"`
	HTTPMaxBodyBytes     int64          `json:"http_max_body_bytes"`
	Targets              []TargetConfig `json:"targets"`
}

type TargetConfig struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Group           string `json:"group"`
	Category        string `json:"category"`
	Target          string `json:"target"`
	Hostname        string `json:"hostname"`
	URL             string `json:"url"`
	Method          string `json:"method"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
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
		ListenAddr:           "0.0.0.0:8080",
		DataPath:             "/var/lib/netwatch/samples.jsonl",
		DataFilePattern:      "samples-%Y-%m-%d.jsonl",
		RetentionDays:        14,
		PingIntervalSeconds:  30,
		PingCount:            10,
		PingTimeoutSeconds:   15,
		DNSIntervalSeconds:   60,
		DNSTimeoutSeconds:    5,
		HTTPIntervalSeconds:  60,
		HTTPTimeoutSeconds:   10,
		HTTPDisableKeepAlive: true,
		HTTPMaxBodyBytes:     262144,
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return errors.New("listen_addr is required")
	}
	if strings.TrimSpace(c.DataPath) == "" && strings.TrimSpace(c.DataDir) == "" {
		return errors.New("data_path or data_dir is required")
	}
	if strings.TrimSpace(c.DataDir) != "" && strings.TrimSpace(c.DataFilePattern) == "" {
		return errors.New("data_file_pattern is required when data_dir is set")
	}
	if c.RetentionDays <= 0 {
		return errors.New("retention_days must be greater than 0")
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
	if c.HTTPMaxBodyBytes <= 0 {
		return errors.New("http_max_body_bytes must be greater than 0")
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
			if err := validateHTTPURL(target.URL); err != nil {
				return fmt.Errorf("targets[%d].url is invalid: %w", i, err)
			}
			if target.Method != "" && strings.ToUpper(target.Method) != "GET" {
				return fmt.Errorf("targets[%d].method must be GET in Phase 3", i)
			}
			if strings.TrimSpace(target.Category) != "" && strings.TrimSpace(target.Group) == "" {
				return fmt.Errorf("targets[%d].group is required when category is set for http target", i)
			}
		default:
			return fmt.Errorf("targets[%d].type must be one of ping, dns, http", i)
		}
		if target.IntervalSeconds < 0 {
			return fmt.Errorf("targets[%d].interval_seconds must be greater than or equal to 0", i)
		}
		if target.TimeoutSeconds < 0 {
			return fmt.Errorf("targets[%d].timeout_seconds must be greater than or equal to 0", i)
		}
	}

	return nil
}

func validateHTTPURL(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("host is required")
	}
	return nil
}

func (c Config) IntervalSeconds(target TargetConfig) int {
	if target.IntervalSeconds > 0 {
		return target.IntervalSeconds
	}
	switch target.Type {
	case "ping":
		return c.PingIntervalSeconds
	case "dns":
		return c.DNSIntervalSeconds
	case "http":
		return c.HTTPIntervalSeconds
	default:
		return c.PingIntervalSeconds
	}
}

func (c Config) TimeoutSeconds(target TargetConfig) int {
	if target.TimeoutSeconds > 0 {
		return target.TimeoutSeconds
	}
	switch target.Type {
	case "ping":
		return c.PingTimeoutSeconds
	case "dns":
		return c.DNSTimeoutSeconds
	case "http":
		return c.HTTPTimeoutSeconds
	default:
		return c.PingTimeoutSeconds
	}
}
