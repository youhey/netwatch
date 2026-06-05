package config

import (
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
	found := false
	for _, target := range cfg.Targets {
		if target.Name == "youtube_home" && target.Group == "youtube" && target.IntervalSeconds == 300 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("youtube_home Phase 3 target not found")
	}
}
