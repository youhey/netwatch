package api

import (
	"strings"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

func TestCollectMonitoringReasonsCoversCodes(t *testing.T) {
	ok := true
	failed := false
	thresholds := config.DefaultMonitoringThresholds()
	samples := []model.Sample{
		{Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0.2), RTTAvgMs: floatPtr(25)},
		{Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(6), RTTAvgMs: floatPtr(220)},
		{Type: "dns", Name: "dns_down", OK: &failed},
		{Type: "dns", Name: "dns_slow", OK: &ok, DurationMs: floatPtr(1200)},
		{Type: "http", Name: "home_down", OK: &failed, Error: "context deadline exceeded"},
		{Type: "http", Name: "home_slow", OK: &ok, TotalMs: floatPtr(6000)},
		{Type: "http", Group: "steam", Name: "steam_store", OK: &failed, Error: "timeout"},
		{Type: "http", Group: "steam", Name: "steam_status", OK: &ok, TotalMs: floatPtr(100)},
		{Type: "download", Name: "r2_1mb", OK: &failed, Error: "timeout"},
		{Type: "download", Name: "r2_10mb", OK: &ok, Mbps: floatPtr(2)},
	}

	reasons := collectMonitoringReasons(samples, thresholds)
	codes := reasonCodeSet(reasons)
	for _, code := range []string{
		"gateway_loss",
		"gateway_rtt_high",
		"packet_loss",
		"external_rtt_high",
		"dns_failure",
		"dns_slow",
		"http_timeout",
		"http_slow",
		"service_failure",
		"service_group_degraded",
		"download_failure",
		"download_slow",
	} {
		if !codes[code] {
			t.Fatalf("reason codes = %+v, want %s", codes, code)
		}
	}
}

func TestBuildMonitoringStatusLevels(t *testing.T) {
	ok := true
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	thresholds := config.DefaultMonitoringThresholds()

	healthy := buildMonitoringStatus([]model.Sample{
		{Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(1)},
	}, thresholds, now)
	if healthy.Alert || healthy.StatusID != "ok" || healthy.PrimaryReason != nil || len(healthy.Reasons) != 0 {
		t.Fatalf("healthy = %+v, want ok without reasons", healthy)
	}

	warning := buildMonitoringStatus([]model.Sample{
		{Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
	}, thresholds, now)
	if !warning.Alert || warning.Level != "warning" || warning.PrimaryReason == nil || warning.PrimaryReason.Code != "download_slow" {
		t.Fatalf("warning = %+v, want download warning", warning)
	}

	critical := buildMonitoringStatus([]model.Sample{
		{Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(220)},
	}, thresholds, now)
	if !critical.Alert || critical.Level != "critical" || critical.PrimaryReason == nil || critical.PrimaryReason.Code != "external_rtt_high" {
		t.Fatalf("critical = %+v, want external RTT critical", critical)
	}
}

func TestSelectPrimaryReasonUsesSeverityPriorityAndBadness(t *testing.T) {
	ok := true
	thresholds := config.DefaultMonitoringThresholds()

	severity := buildMonitoringStatus([]model.Sample{
		{Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(2)},
		{Type: "download", Name: "r2_10mb", OK: &ok, Mbps: floatPtr(2)},
	}, thresholds, time.Now())
	if severity.PrimaryReason == nil || severity.PrimaryReason.Code != "download_slow" {
		t.Fatalf("primary = %+v, want critical download_slow before warning packet_loss", severity.PrimaryReason)
	}

	priority := buildMonitoringStatus([]model.Sample{
		{Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(2)},
		{Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
	}, thresholds, time.Now())
	if priority.PrimaryReason == nil || priority.PrimaryReason.Code != "packet_loss" {
		t.Fatalf("primary = %+v, want packet_loss by reason priority", priority.PrimaryReason)
	}

	badness := buildMonitoringStatus([]model.Sample{
		{Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(2)},
		{Type: "ping", Name: "google_dns", OK: &ok, LossPercent: floatPtr(3)},
	}, thresholds, time.Now())
	if badness.PrimaryReason == nil || badness.PrimaryReason.Target != "google_dns" {
		t.Fatalf("primary = %+v, want worse packet loss", badness.PrimaryReason)
	}
}

func TestMonitoringStatusIDIsStableForSameReasons(t *testing.T) {
	ok := true
	thresholds := config.DefaultMonitoringThresholds()
	samples := []model.Sample{
		{Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
	}

	first := buildMonitoringStatus(samples, thresholds, time.Now())
	second := buildMonitoringStatus(samples, thresholds, time.Now().Add(time.Minute))
	if first.StatusID == "" || first.StatusID != second.StatusID || strings.Contains(first.StatusID, "2026") {
		t.Fatalf("status_id first=%q second=%q, want stable non-timestamp id", first.StatusID, second.StatusID)
	}
}

func reasonCodeSet(reasons []monitoringReason) map[string]bool {
	codes := make(map[string]bool, len(reasons))
	for _, reason := range reasons {
		codes[reason.Code] = true
	}
	return codes
}
