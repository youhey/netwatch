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
	ListenAddr           string                `json:"listen_addr"`
	DataPath             string                `json:"data_path"`
	DataDir              string                `json:"data_dir"`
	DataFilePattern      string                `json:"data_file_pattern"`
	RetentionDays        int                   `json:"retention_days"`
	PingIntervalSeconds  int                   `json:"ping_interval_seconds"`
	PingCount            int                   `json:"ping_count"`
	PingTimeoutSeconds   int                   `json:"ping_timeout_seconds"`
	DNSIntervalSeconds   int                   `json:"dns_interval_seconds"`
	DNSTimeoutSeconds    int                   `json:"dns_timeout_seconds"`
	HTTPIntervalSeconds  int                   `json:"http_interval_seconds"`
	HTTPTimeoutSeconds   int                   `json:"http_timeout_seconds"`
	HTTPDisableKeepAlive bool                  `json:"http_disable_keepalive"`
	HTTPMaxBodyBytes     int64                 `json:"http_max_body_bytes"`
	MonitoringThresholds MonitoringThresholds  `json:"monitoring_thresholds"`
	DownloadProbes       []DownloadProbeConfig `json:"download_probes"`
	Targets              []TargetConfig        `json:"targets"`
}

type Threshold struct {
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
}

type MonitoringThresholds struct {
	Ping     PingThresholds       `json:"ping"`
	DNS      DNSThresholds        `json:"dns"`
	HTTP     HTTPThresholds       `json:"http"`
	Download map[string]Threshold `json:"download"`
	Service  ServiceThresholds    `json:"service"`
}

type PingThresholds struct {
	GatewayRTTAvgMs     Threshold `json:"gateway_rtt_avg_ms"`
	GatewayLossPercent  Threshold `json:"gateway_loss_percent"`
	ExternalRTTAvgMs    Threshold `json:"external_rtt_avg_ms"`
	ExternalLossPercent Threshold `json:"external_loss_percent"`
}

type DNSThresholds struct {
	DurationMs Threshold `json:"duration_ms"`
}

type HTTPThresholds struct {
	TotalMs Threshold `json:"total_ms"`
}

type ServiceThresholds struct {
	OKRatePercent Threshold `json:"ok_rate_percent"`
}

type TargetConfig struct {
	Name            string `json:"name"`
	Label           string `json:"label"`
	Type            string `json:"type"`
	Group           string `json:"group"`
	Category        string `json:"category"`
	Target          string `json:"target"`
	Hostname        string `json:"hostname"`
	URL             string `json:"url"`
	Method          string `json:"method"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	DisplayOrder    int    `json:"display_order"`
}

type DownloadProbeConfig struct {
	Name            string `json:"name"`
	Label           string `json:"label"`
	URL             string `json:"url"`
	ExpectedBytes   int64  `json:"expected_bytes"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	Enabled         bool   `json:"enabled"`
	DisplayOrder    int    `json:"display_order"`
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
		MonitoringThresholds: DefaultMonitoringThresholds(),
	}
}

func DefaultMonitoringThresholds() MonitoringThresholds {
	return MonitoringThresholds{
		Ping: PingThresholds{
			GatewayRTTAvgMs:     Threshold{Warning: 5, Critical: 20},
			GatewayLossPercent:  Threshold{Warning: 0.1, Critical: 1},
			ExternalRTTAvgMs:    Threshold{Warning: 100, Critical: 200},
			ExternalLossPercent: Threshold{Warning: 1, Critical: 5},
		},
		DNS: DNSThresholds{
			DurationMs: Threshold{Warning: 300, Critical: 1000},
		},
		HTTP: HTTPThresholds{
			TotalMs: Threshold{Warning: 3000, Critical: 5000},
		},
		Download: map[string]Threshold{
			"r2_1mb_mbps":  {Warning: 5, Critical: 1},
			"r2_10mb_mbps": {Warning: 10, Critical: 3},
		},
		Service: ServiceThresholds{
			OKRatePercent: Threshold{Warning: 95, Critical: 90},
		},
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
	if err := c.MonitoringThresholds.Validate(); err != nil {
		return err
	}
	if len(c.Targets) == 0 {
		return errors.New("targets must not be empty")
	}

	names := make(map[string]struct{}, len(c.Targets)+len(c.DownloadProbes))
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
		if target.DisplayOrder < 0 {
			return fmt.Errorf("targets[%d].display_order must be greater than or equal to 0", i)
		}
	}

	for i, downloadProbe := range c.DownloadProbes {
		if !downloadProbe.Enabled {
			continue
		}
		if strings.TrimSpace(downloadProbe.Name) == "" {
			return fmt.Errorf("download_probes[%d].name is required", i)
		}
		if _, ok := names[downloadProbe.Name]; ok {
			return fmt.Errorf("duplicate target name: %s", downloadProbe.Name)
		}
		names[downloadProbe.Name] = struct{}{}
		if strings.TrimSpace(downloadProbe.URL) == "" {
			return fmt.Errorf("download_probes[%d].url is required", i)
		}
		if err := validateHTTPURL(downloadProbe.URL); err != nil {
			return fmt.Errorf("download_probes[%d].url is invalid: %w", i, err)
		}
		if downloadProbe.ExpectedBytes < 0 {
			return fmt.Errorf("download_probes[%d].expected_bytes must be greater than or equal to 0", i)
		}
		if downloadProbe.IntervalSeconds <= 0 {
			return fmt.Errorf("download_probes[%d].interval_seconds must be greater than 0", i)
		}
		if downloadProbe.TimeoutSeconds <= 0 {
			return fmt.Errorf("download_probes[%d].timeout_seconds must be greater than 0", i)
		}
		if downloadProbe.DisplayOrder < 0 {
			return fmt.Errorf("download_probes[%d].display_order must be greater than or equal to 0", i)
		}
	}

	return nil
}

func (t MonitoringThresholds) Validate() error {
	if err := validateHighBadThreshold("monitoring_thresholds.ping.gateway_rtt_avg_ms", t.Ping.GatewayRTTAvgMs); err != nil {
		return err
	}
	if err := validateHighBadThreshold("monitoring_thresholds.ping.gateway_loss_percent", t.Ping.GatewayLossPercent); err != nil {
		return err
	}
	if err := validateHighBadThreshold("monitoring_thresholds.ping.external_rtt_avg_ms", t.Ping.ExternalRTTAvgMs); err != nil {
		return err
	}
	if err := validateHighBadThreshold("monitoring_thresholds.ping.external_loss_percent", t.Ping.ExternalLossPercent); err != nil {
		return err
	}
	if err := validateHighBadThreshold("monitoring_thresholds.dns.duration_ms", t.DNS.DurationMs); err != nil {
		return err
	}
	if err := validateHighBadThreshold("monitoring_thresholds.http.total_ms", t.HTTP.TotalMs); err != nil {
		return err
	}
	for name, threshold := range t.Download {
		if err := validateLowBadThreshold("monitoring_thresholds.download."+name, threshold); err != nil {
			return err
		}
	}
	if err := validateLowBadThreshold("monitoring_thresholds.service.ok_rate_percent", t.Service.OKRatePercent); err != nil {
		return err
	}
	return nil
}

func validateHighBadThreshold(name string, threshold Threshold) error {
	if threshold.Warning <= 0 || threshold.Critical <= 0 {
		return fmt.Errorf("%s warning and critical must be greater than 0", name)
	}
	if threshold.Critical < threshold.Warning {
		return fmt.Errorf("%s critical must be greater than or equal to warning", name)
	}
	return nil
}

func validateLowBadThreshold(name string, threshold Threshold) error {
	if threshold.Warning <= 0 || threshold.Critical <= 0 {
		return fmt.Errorf("%s warning and critical must be greater than 0", name)
	}
	if threshold.Critical > threshold.Warning {
		return fmt.Errorf("%s critical must be less than or equal to warning", name)
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

func (c Config) EnabledDownloadProbes() []DownloadProbeConfig {
	probes := make([]DownloadProbeConfig, 0, len(c.DownloadProbes))
	for _, probe := range c.DownloadProbes {
		if !probe.Enabled {
			continue
		}
		probes = append(probes, probe)
	}
	return probes
}
