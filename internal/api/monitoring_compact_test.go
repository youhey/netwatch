package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/model"
)

func TestMonitoringCompactOK(t *testing.T) {
	state := collector.NewState()
	ok := true
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(1)},
		{Timestamp: now, Type: "status_page", Name: "github_status", DisplayName: "GitHub Status", OK: &ok, Level: "ok", Indicator: "none", Description: "All Systems Operational"},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/compact", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body monitoringCompactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Source != "netwatch" || body.Level != "ok" || body.Label != "NET OK" || body.Alert || body.Title != "All systems operational" || body.Message != "All probes are healthy." {
		t.Fatalf("body = %+v, want compact OK response", body)
	}
	if body.IssueCount != 0 || body.PrimaryReason != nil {
		t.Fatalf("body = %+v, want no issue reason", body)
	}
	if body.History.Range != "2h" || body.History.Bucket != "5m" || body.History.BucketSeconds != 300 || len(body.History.Points) != 24 {
		t.Fatalf("history = %+v, want compact 2h/5m history", body.History)
	}
	if body.History.Points[0].Level == "" {
		t.Fatalf("history point = %+v, want level", body.History.Points[0])
	}
	if body.ProviderStatus.Level != "ok" || body.ProviderStatus.Alert || body.ProviderStatus.IssueCount != 0 || len(body.ProviderStatus.Providers) != 1 || body.ProviderStatus.Providers[0].Name != "github_status" {
		t.Fatalf("provider_status = %+v, want compact provider status", body.ProviderStatus)
	}
}

func TestMonitoringCompactProviderStatusWarning(t *testing.T) {
	state := collector.NewState()
	failed := false
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "status_page", Name: "openai_status", DisplayName: "OpenAI Status", OK: &failed, Level: "warning", Indicator: "minor", Description: "Partial System Outage"},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/compact", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringCompactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.ProviderStatus.Level != "warning" || !body.ProviderStatus.Alert || body.ProviderStatus.IssueCount != 1 || len(body.ProviderStatus.Providers) != 1 {
		t.Fatalf("provider_status = %+v, want warning alert", body.ProviderStatus)
	}
}

func TestMonitoringCompactWarningReason(t *testing.T) {
	state := collector.NewState()
	ok := true
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/compact", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringCompactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Level != "warning" || body.Label != "WARN" || !body.Alert || body.IssueCount != 1 {
		t.Fatalf("body = %+v, want compact warning response", body)
	}
	if body.PrimaryReason == nil || body.PrimaryReason.Code != "download_slow" || body.PrimaryReason.Target != "r2_1mb" || body.PrimaryReason.Metric != "mbps" || body.PrimaryReason.Value != 3.2 {
		t.Fatalf("primary_reason = %+v, want minimal download_slow reason", body.PrimaryReason)
	}
	if body.Title != "Network degradation detected" || body.Message != "Download throughput is below the warning threshold on r2_1mb." {
		t.Fatalf("body = %+v, want warning title/message", body)
	}
	if len(body.History.Points) != compactHistoryPoints {
		t.Fatalf("len(history.points) = %d, want %d", len(body.History.Points), compactHistoryPoints)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	var status monitoringStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Unmarshal(status) error = %v", err)
	}
	if body.Level != status.Level || body.IssueCount != len(status.Reasons) {
		t.Fatalf("compact = %+v status = %+v, want matching level and issue count", body, status)
	}
}

func TestMonitoringCompactCriticalAndUnknownLabels(t *testing.T) {
	critical := buildMonitoringCompact(monitoringStatusResponse{
		Source:  "netwatch",
		Level:   "critical",
		Alert:   true,
		Reasons: []monitoringReason{{Code: "packet_loss"}},
		PrimaryReason: &monitoringReason{
			Code:   "packet_loss",
			Level:  "critical",
			Target: "cloudflare_dns",
			Metric: "loss_percent",
			Value:  6,
		},
	}, emptyCompactHistory(), time.Now())
	if critical.Label != "CRIT" || critical.Title != "Critical network issue detected" || critical.IssueCount != 1 {
		t.Fatalf("critical = %+v, want compact critical response", critical)
	}

	unknown := buildMonitoringCompact(monitoringStatusResponse{
		Source: "netwatch",
		Level:  "unknown",
		Alert:  true,
	}, emptyCompactHistory(), time.Now())
	if unknown.Label != "UNK" || unknown.PrimaryReason != nil || unknown.Message != "Netwatch cannot determine current network health." {
		t.Fatalf("unknown = %+v, want compact unknown response", unknown)
	}
}

func TestMonitoringCompactCapabilities(t *testing.T) {
	handler := New(collector.NewState(), "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body struct {
		Features map[string]bool `json:"features"`
		Compact  struct {
			HistoryRange  string `json:"history_range"`
			HistoryBucket string `json:"history_bucket"`
			HistoryPoints int    `json:"history_points"`
		} `json:"monitoring_compact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !body.Features["monitoring_compact"] || body.Compact.HistoryRange != "2h" || body.Compact.HistoryBucket != "5m" || body.Compact.HistoryPoints != 24 {
		t.Fatalf("capabilities = %+v, want compact support", body)
	}
}

func emptyCompactHistory() monitoringStatusHistoryResponse {
	return monitoringStatusHistoryResponse{
		Range:         "2h",
		Bucket:        "5m",
		BucketSeconds: 300,
		Points:        []monitoringStatusHistoryPoint{},
	}
}
