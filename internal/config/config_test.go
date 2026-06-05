package config

import "testing"

func TestValidatePhase2Targets(t *testing.T) {
	cfg := Default()
	cfg.Targets = []TargetConfig{
		{Name: "gateway", Type: "ping", Target: "192.168.1.1"},
		{Name: "lookup", Type: "dns", Hostname: "www.google.com"},
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
