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
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "interval_seconds": 600,
      "timeout_seconds": 20,
      "enabled": true
    },
    {
      "name": "disabled",
      "enabled": false
    }
  ],
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
	if len(cfg.DownloadProbes) != 2 {
		t.Fatalf("len(DownloadProbes) = %d, want 2", len(cfg.DownloadProbes))
	}
	probes := cfg.EnabledDownloadProbes()
	if len(probes) != 1 {
		t.Fatalf("len(EnabledDownloadProbes) = %d, want 1", len(probes))
	}
	probe := probes[0]
	if probe.Name != "r2_1mb" || probe.ExpectedBytes != 1048576 || probe.IntervalSeconds != 600 || probe.TimeoutSeconds != 20 {
		t.Fatalf("probe = %+v, want download settings loaded", probe)
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
	for _, target := range cfg.Targets {
		if target.Name == "youtube_home" && target.Group == "youtube" && target.IntervalSeconds == 300 {
			found = true
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
}
