package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePhase2Targets(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "gateway", Type: "ping", Target: "192.168.1.1"},
		{Name: "lookup", Type: "dns", Hostname: "www.google.com"},
		{Name: "home", Type: "http", URL: "https://example.com/"},
		{Name: "youtube", Type: "http", Group: "youtube", Category: "service", URL: "https://www.youtube.com/", IntervalSeconds: 300, TimeoutSeconds: 8},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestTargetIntervalAndTimeoutOverride(t *testing.T) {
	cfg := Default()
	target := TargetConfig{
		Name:            "youtube",
		Type:            "http",
		Group:           "youtube",
		Category:        "service",
		URL:             "https://www.youtube.com/",
		IntervalSeconds: 300,
		TimeoutSeconds:  8,
	}

	if got := cfg.IntervalSeconds(target); got != 300 {
		t.Fatalf("IntervalSeconds() = %d, want 300", got)
	}
	if got := cfg.TimeoutSeconds(target); got != 8 {
		t.Fatalf("TimeoutSeconds() = %d, want 8", got)
	}
}

func TestTargetIntervalAndTimeoutDefault(t *testing.T) {
	cfg := Default()
	target := TargetConfig{Name: "home", Type: "http", URL: "https://example.com/"}

	if got := cfg.IntervalSeconds(target); got != cfg.HTTPIntervalSeconds {
		t.Fatalf("IntervalSeconds() = %d, want %d", got, cfg.HTTPIntervalSeconds)
	}
	if got := cfg.TimeoutSeconds(target); got != cfg.HTTPTimeoutSeconds {
		t.Fatalf("TimeoutSeconds() = %d, want %d", got, cfg.HTTPTimeoutSeconds)
	}
}

func TestValidatePhase35Settings(t *testing.T) {
	cfg := Default()
	cfg.DataPath = ""
	cfg.DataDir = "/var/lib/netwatch"
	cfg.DataFilePattern = "samples-%Y-%m-%d.jsonl"
	cfg.RetentionDays = 14
	cfg.HTTPDisableKeepAlive = true
	cfg.HTTPMaxBodyBytes = 262144
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http", Group: "baseline", Category: "baseline", URL: "https://example.com/"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadPhase35Settings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netwatch.json")
	content := `{
  "listen_addr": "127.0.0.1:8080",
  "data_dir": "/var/lib/netwatch",
  "data_file_pattern": "samples-%Y-%m-%d.jsonl",
  "retention_days": 7,
  "http_disable_keepalive": false,
  "http_max_body_bytes": 131072,
  "targets": [
    {
      "name": "home",
      "type": "http",
      "group": "baseline",
      "category": "baseline",
      "url": "https://example.com/"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DataDir != "/var/lib/netwatch" || cfg.RetentionDays != 7 || cfg.HTTPDisableKeepAlive || cfg.HTTPMaxBodyBytes != 131072 {
		t.Fatalf("cfg = %+v, want Phase 3.5 settings loaded", cfg)
	}
}

func TestLoadDownloadProbes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netwatch.json")
	content := `{
  "listen_addr": "127.0.0.1:8080",
  "data_dir": "/var/lib/netwatch",
  "data_file_pattern": "samples-%Y-%m-%d.jsonl",
  "retention_days": 7,
  "download_probes": [
    {
      "name": "r2_1mb",
      "label": "R2 1MB",
      "display_order": 10,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "interval_seconds": 600,
      "timeout_seconds": 20,
      "enabled": true,
      "retry_on_alert": {
        "enabled": true,
        "intervals_seconds": [10, 30, 60],
        "recovery_success_count": 3
      }
    },
    {
      "name": "disabled",
      "enabled": false
    }
  ],
  "targets": [
    {
      "name": "home",
      "label": "Home",
      "display_order": 70,
      "type": "http",
      "url": "https://example.com/"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.DownloadProbes) != 2 {
		t.Fatalf("len(DownloadProbes) = %d, want 2", len(cfg.DownloadProbes))
	}
	probes := cfg.EnabledDownloadProbes()
	if len(probes) != 1 {
		t.Fatalf("len(EnabledDownloadProbes) = %d, want 1", len(probes))
	}
	probe := probes[0]
	if probe.Name != "r2_1mb" || probe.Label != "R2 1MB" || probe.DisplayOrder != 10 || probe.ExpectedBytes != 1048576 || probe.IntervalSeconds != 600 || probe.TimeoutSeconds != 20 {
		t.Fatalf("probe = %+v, want download settings loaded", probe)
	}
	retry := probe.EffectiveRetryOnAlert()
	if !retry.Enabled || len(retry.IntervalsSeconds) != 3 || retry.IntervalsSeconds[0] != 10 || retry.IntervalsSeconds[2] != 60 || retry.RecoverySuccessCount != 3 {
		t.Fatalf("retry = %+v, want retry_on_alert settings loaded", retry)
	}
	if cfg.Targets[0].Label != "Home" || cfg.Targets[0].DisplayOrder != 70 {
		t.Fatalf("target = %+v, want label and display_order", cfg.Targets[0])
	}
}

func TestDownloadRetryOnAlertDefaults(t *testing.T) {
	probe := DownloadProbeConfig{
		Name:            "r2_10mb",
		IntervalSeconds: 3600,
		RetryOnAlert: RetryOnAlertConfig{
			Enabled: true,
		},
	}

	retry := probe.EffectiveRetryOnAlert()
	if !retry.Enabled || retry.RecoverySuccessCount != 2 {
		t.Fatalf("retry = %+v, want enabled default recovery count", retry)
	}
	if len(retry.IntervalsSeconds) != 7 || retry.IntervalsSeconds[0] != 30 || retry.IntervalsSeconds[6] != 3600 {
		t.Fatalf("intervals = %+v, want r2_10mb defaults", retry.IntervalsSeconds)
	}
}

func TestDownloadRetryOnAlertDefaultDisabledForExistingConfig(t *testing.T) {
	probe := DownloadProbeConfig{
		Name:            "r2_1mb",
		IntervalSeconds: 600,
	}

	retry := probe.EffectiveRetryOnAlert()
	if retry.Enabled || len(retry.IntervalsSeconds) != 0 || retry.RecoverySuccessCount != 0 {
		t.Fatalf("retry = %+v, want disabled zero-value for existing config", retry)
	}
}

func TestDefaultMonitoringThresholds(t *testing.T) {
	thresholds := DefaultMonitoringThresholds()
	if thresholds.Ping.GatewayRTTAvgMs.Warning != 5 || thresholds.Ping.GatewayRTTAvgMs.Critical != 20 {
		t.Fatalf("gateway RTT threshold = %+v, want default", thresholds.Ping.GatewayRTTAvgMs)
	}
	if thresholds.Download["r2_1mb_mbps"].Warning != 5 || thresholds.Download["r2_1mb_mbps"].Critical != 1 {
		t.Fatalf("r2_1mb threshold = %+v, want default", thresholds.Download["r2_1mb_mbps"])
	}
	if thresholds.Service.OKRatePercent.Warning != 95 || thresholds.Service.OKRatePercent.Critical != 90 {
		t.Fatalf("service threshold = %+v, want default", thresholds.Service.OKRatePercent)
	}
}

func TestLoadMonitoringThresholds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netwatch.json")
	content := `{
  "listen_addr": "127.0.0.1:8080",
  "data_dir": "/var/lib/netwatch",
  "data_file_pattern": "samples-%Y-%m-%d.jsonl",
  "retention_days": 7,
  "monitoring_thresholds": {
    "http": {
      "total_ms": {"warning": 2500, "critical": 4500}
    },
    "download": {
      "r2_1mb_mbps": {"warning": 8, "critical": 2}
    }
  },
  "targets": [
    {
      "name": "home",
      "type": "http",
      "url": "https://example.com/"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MonitoringThresholds.HTTP.TotalMs.Warning != 2500 || cfg.MonitoringThresholds.HTTP.TotalMs.Critical != 4500 {
		t.Fatalf("http threshold = %+v, want configured values", cfg.MonitoringThresholds.HTTP.TotalMs)
	}
	if cfg.MonitoringThresholds.Download["r2_1mb_mbps"].Warning != 8 || cfg.MonitoringThresholds.Download["r2_10mb_mbps"].Warning != 10 {
		t.Fatalf("download thresholds = %+v, want configured value with default retained", cfg.MonitoringThresholds.Download)
	}
	if cfg.MonitoringThresholds.Ping.ExternalRTTAvgMs.Warning != 100 {
		t.Fatalf("ping threshold = %+v, want default retained", cfg.MonitoringThresholds.Ping.ExternalRTTAvgMs)
	}
}

func TestValidateMonitoringThresholds(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http", URL: "https://example.com/"},
	}

	cfg.MonitoringThresholds.HTTP.TotalMs = Threshold{Warning: 5000, Critical: 3000}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid high-bad threshold error")
	}

	cfg = Default()
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http", URL: "https://example.com/"},
	}
	cfg.MonitoringThresholds.Download["r2_1mb_mbps"] = Threshold{Warning: 1, Critical: 5}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid low-bad threshold error")
	}
}

func TestValidateDownloadProbeURL(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http", URL: "https://example.com/"},
	}
	cfg.DownloadProbes = []DownloadProbeConfig{
		{Name: "r2_1mb", URL: "ftp://example.com/file.bin", ExpectedBytes: 1, IntervalSeconds: 600, TimeoutSeconds: 20, Enabled: true},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateLegacyDataPath(t *testing.T) {
	cfg := Default()
	cfg.DataPath = "/var/lib/netwatch/samples.jsonl"
	cfg.DataDir = ""
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http", URL: "https://example.com/"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateUnknownTargetType(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "bad", Type: "tcp", Target: "example.com"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateDNSRequiresHostname(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "lookup", Type: "dns"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateHTTPRequiresURL(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "home", Type: "http"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateHTTPRequiresGroupWhenCategoryIsSet(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "youtube", Type: "http", Category: "service", URL: "https://www.youtube.com/"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateHTTPURL(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "bad", Type: "http", URL: "ftp://example.com/"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "netwatch.example.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Targets) < 10 {
		t.Fatalf("len(Targets) = %d, want Phase 3 service targets", len(cfg.Targets))
	}
	if len(cfg.EnabledDownloadProbes()) != 2 {
		t.Fatalf("len(EnabledDownloadProbes) = %d, want Phase 5 download probes", len(cfg.EnabledDownloadProbes()))
	}
	found := false
	foundSF6 := false
	foundGatewayOrder := false
	foundGoogleOrder := false
	for _, target := range cfg.Targets {
		if target.Name == "youtube_home" && target.Label == "YouTube Home" && target.Group == "youtube" && target.IntervalSeconds == 300 && target.DisplayOrder == 10 {
			found = true
		}
		if target.Name == "gateway" && target.Label == "Gateway" && target.DisplayOrder == 10 {
			foundGatewayOrder = true
		}
		if target.Name == "google_dns" && target.Label == "Google DNS" && target.DisplayOrder == 20 {
			foundGoogleOrder = true
		}
		if target.Name == "sf6_buckler_info" {
			foundSF6 = true
		}
	}
	if !found {
		t.Fatal("youtube_home Phase 3 target not found")
	}
	if foundSF6 {
		t.Fatal("sf6_buckler_info should not be in example config")
	}
	if !foundGatewayOrder || !foundGoogleOrder {
		t.Fatal("example config display_order for ping targets not found")
	}
	probes := cfg.EnabledDownloadProbes()
	if probes[0].Name != "r2_1mb" || probes[0].Label != "R2 1MB" || probes[0].DisplayOrder != 10 || probes[1].Name != "r2_10mb" || probes[1].Label != "R2 10MB" || probes[1].DisplayOrder != 20 {
		t.Fatalf("download probe order = %+v, want r2_1mb then r2_10mb", probes)
	}
	firstRetry := probes[0].EffectiveRetryOnAlert()
	secondRetry := probes[1].EffectiveRetryOnAlert()
	if !firstRetry.Enabled || len(firstRetry.IntervalsSeconds) != 6 || firstRetry.IntervalsSeconds[0] != 10 || firstRetry.RecoverySuccessCount != 2 {
		t.Fatalf("r2_1mb retry = %+v, want example adaptive retry", firstRetry)
	}
	if !secondRetry.Enabled || len(secondRetry.IntervalsSeconds) != 7 || secondRetry.IntervalsSeconds[0] != 30 || secondRetry.IntervalsSeconds[6] != 3600 || secondRetry.RecoverySuccessCount != 2 {
		t.Fatalf("r2_10mb retry = %+v, want example adaptive retry", secondRetry)
	}
}
