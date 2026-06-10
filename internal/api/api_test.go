package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

func TestParseRange(t *testing.T) {
	tests := map[string]int{
		"1h":  1,
		"6h":  6,
		"24h": 24,
		"7d":  24 * 7,
		"14d": 24 * 14,
	}

	for value, wantHours := range tests {
		got, err := parseRange(value)
		if err != nil {
			t.Fatalf("parseRange(%q) error = %v", value, err)
		}
		if int(got.Hours()) != wantHours {
			t.Fatalf("parseRange(%q) = %v hours, want %d", value, got.Hours(), wantHours)
		}
	}
}

func TestParseRangeUnsupported(t *testing.T) {
	if _, err := parseRange("30m"); err == nil {
		t.Fatal("parseRange() error = nil, want error")
	}
}

func TestLatestGroupsSamplesByType(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string][]model.Sample
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body["ping"]) != 1 || len(body["dns"]) != 1 || len(body["http"]) != 4 || len(body["download"]) != 1 {
		t.Fatalf("counts = ping:%d dns:%d http:%d download:%d, want ping:1 dns:1 http:4 download:1", len(body["ping"]), len(body["dns"]), len(body["http"]), len(body["download"]))
	}
	if body["download"][0].DisplayOrder != 10 {
		t.Fatalf("download display_order = %d, want 10", body["download"][0].DisplayOrder)
	}
	if body["download"][0].DisplayName != "R2 1MB" {
		t.Fatalf("download display_name = %q, want R2 1MB", body["download"][0].DisplayName)
	}
}

func TestDNSLatest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/dns/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertSampleCount(t, rec, 1)
}

func TestHTTPLatest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/http/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertSampleCount(t, rec, 4)
}

func TestDownloadLatest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/download/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertSampleCount(t, rec, 1)
}

func TestStatusPagesLatest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/status-pages/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusPagesLatestBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Providers) != 1 {
		t.Fatalf("len(providers) = %d, want 1", len(body.Providers))
	}
	provider := body.Providers[0]
	if provider.Name != "github_status" || provider.Label != "GitHub Status" || provider.Level != "ok" || !provider.OK || provider.Indicator != "none" || len(provider.Components) != 1 {
		t.Fatalf("provider = %+v, want github status page", provider)
	}
}

func TestSummaryIncludesProviderStatus(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		NetworkStatus  monitoringSummaryResponse     `json:"network_status"`
		ServiceHealth  serviceHealthSummaryResponse  `json:"service_health"`
		ProviderStatus providerStatusSummaryResponse `json:"provider_status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.NetworkStatus.Level != "ok" || body.NetworkStatus.Alert || body.NetworkStatus.IssueCount != 0 {
		t.Fatalf("network_status = %+v, want ok network summary", body.NetworkStatus)
	}
	if body.ServiceHealth.Level != "warning" || body.ServiceHealth.Alert || body.ServiceHealth.IssueCount != 1 || body.ServiceHealth.Groups != 2 || body.ServiceHealth.Services != 2 {
		t.Fatalf("service_health = %+v, want service summary", body.ServiceHealth)
	}
	if body.ProviderStatus.Level != "ok" || !body.ProviderStatus.OK || body.ProviderStatus.Alert || body.ProviderStatus.IssueCount != 0 || body.ProviderStatus.Providers != 1 {
		t.Fatalf("provider_status = %+v, want ok summary", body.ProviderStatus)
	}
}

func TestDownloadLatestIncludesRetryMetadata(t *testing.T) {
	state := collector.NewState()
	ok := true
	nextCheckAt := time.Date(2026, 6, 7, 13, 30, 30, 0, time.UTC)
	state.Load([]model.Sample{
		{
			Timestamp:            time.Now(),
			Type:                 "download",
			Name:                 "r2_1mb",
			OK:                   &ok,
			Mbps:                 floatPtr(3.2),
			RetryState:           "degraded",
			RetryAttempt:         intPtr(1),
			RecoverySuccessCount: intPtr(0),
			NextCheckAt:          &nextCheckAt,
		},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/download/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body struct {
		Samples []model.Sample `json:"samples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(body.Samples))
	}
	sample := body.Samples[0]
	if sample.RetryState != "degraded" || sample.RetryAttempt == nil || *sample.RetryAttempt != 1 || sample.RecoverySuccessCount == nil || *sample.RecoverySuccessCount != 0 || sample.NextCheckAt == nil || !sample.NextCheckAt.Equal(nextCheckAt) {
		t.Fatalf("sample = %+v, want retry metadata", sample)
	}
}

func TestLatestBackfillsDisplayOrderFromConfig(t *testing.T) {
	state := collector.NewState()
	ok := true
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok},
		{Timestamp: time.Now(), Type: "ping", Name: "gateway", OK: &ok},
		{Timestamp: time.Now(), Type: "ping", Name: "google_dns", OK: &ok},
	})
	targets := []config.TargetConfig{
		{Name: "gateway", Label: "Gateway", DisplayOrder: 10, Type: "ping", Target: "192.168.1.1"},
		{Name: "google_dns", Label: "Google DNS", DisplayOrder: 20, Type: "ping", Target: "8.8.8.8"},
		{Name: "cloudflare_dns", Label: "Cloudflare DNS", DisplayOrder: 30, Type: "ping", Target: "1.1.1.1"},
	}
	handler := New(state, "test", targets).Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/ping/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body struct {
		Samples []model.Sample `json:"samples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Samples[0].Name != "gateway" || body.Samples[0].DisplayName != "Gateway" || body.Samples[0].DisplayOrder != 10 || body.Samples[1].Name != "google_dns" || body.Samples[1].DisplayName != "Google DNS" || body.Samples[2].Name != "cloudflare_dns" {
		t.Fatalf("samples = %+v, want config display order", body.Samples)
	}
}

func TestServicesLatest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/services/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Services []serviceGroupResponse `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Services) != 2 {
		t.Fatalf("len(services) = %d, want 2", len(body.Services))
	}
	if body.Services[0].Group != "steam" || body.Services[0].DisplayName != "Steam" || body.Services[0].Status != "warning" {
		t.Fatalf("services[0] = %+v, want steam warning", body.Services[0])
	}
	if body.Services[1].Group != "youtube" || body.Services[1].DisplayName != "Youtube" || body.Services[1].Status != "ok" {
		t.Fatalf("services[1] = %+v, want youtube ok", body.Services[1])
	}
}

func TestServicesSeriesByGroup(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/services/series?group=youtube&range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertSampleCount(t, rec, 2)
}

func TestPingSeriesWithBucketReturnsChart(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ping/series?name=cloudflare_dns&range=24h&bucket=5m&max_points=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Type != "ping" || body.Name != "cloudflare_dns" || body.Bucket != "5m" || len(body.Points) != 1 {
		t.Fatalf("body = %+v, want ping chart response", body)
	}
}

func TestHTTPSeriesWithBucketReturnsChart(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/http/series?name=youtube_home&range=24h&bucket=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Type != "http" || body.Name != "youtube_home" || len(body.Points) == 0 {
		t.Fatalf("body = %+v, want http chart response", body)
	}
}

func TestDownloadSeriesReturnsPoints(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/download/series?name=r2_1mb&range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Name   string         `json:"name"`
		Range  string         `json:"range"`
		Points []model.Sample `json:"points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Name != "r2_1mb" || body.Range != "24h" || len(body.Points) != 1 {
		t.Fatalf("body = %+v, want download points", body)
	}
}

func TestDownloadSeriesWithBucketReturnsChart(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/download/series?name=r2_1mb&range=24h&bucket=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Type != "download" || body.Name != "r2_1mb" || len(body.Points) != 1 || body.Points[0].AvgMbps == nil {
		t.Fatalf("body = %+v, want download chart response", body)
	}
}

func TestServicesSeriesWithBucketReturnsChart(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/services/series?group=youtube&range=24h&bucket=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Type != "service_group" || body.Group != "youtube" || len(body.Targets) != 1 || len(body.Points) == 0 {
		t.Fatalf("body = %+v, want service chart response", body)
	}
}

func TestSeriesWithInvalidBucketReturnsBadRequest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ping/series?name=cloudflare_dns&range=24h&bucket=2m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSeriesWithInvalidMaxPointsReturnsBadRequest(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ping/series?name=cloudflare_dns&range=24h&bucket=5m&max_points=3", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSeriesWithMissingTargetReturnsStructuredNotFound(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ping/series?name=missing&range=24h&bucket=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var body struct {
		Error struct {
			Code  string `json:"code"`
			Param string `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Error.Code != "target_not_found" || body.Error.Param != "name" {
		t.Fatalf("error = %+v, want target_not_found/name", body.Error)
	}
}

func TestChartsCatalog(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/charts/catalog", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Ping          []catalogTarget       `json:"ping"`
		DNS           []catalogTarget       `json:"dns"`
		HTTP          []catalogTarget       `json:"http"`
		Download      []catalogTarget       `json:"download"`
		ServiceGroups []catalogServiceGroup `json:"service_groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Ping) != 3 || len(body.DNS) != 1 || len(body.HTTP) != 2 || len(body.Download) != 2 || len(body.ServiceGroups) != 2 {
		t.Fatalf("catalog counts = ping:%d dns:%d http:%d download:%d groups:%d", len(body.Ping), len(body.DNS), len(body.HTTP), len(body.Download), len(body.ServiceGroups))
	}
	if body.Ping[0].Name != "gateway" || body.Ping[0].DisplayName != "Gateway" || body.Ping[1].Name != "google_dns" || body.Ping[1].DisplayName != "Google DNS" || body.Ping[2].Name != "cloudflare_dns" {
		t.Fatalf("ping catalog = %+v, want display order", body.Ping)
	}
	if body.Download[0].Name != "r2_1mb" || body.Download[0].DisplayName != "R2 1MB" || body.Download[0].DisplayOrder != 10 || body.Download[1].Name != "r2_10mb" || body.Download[1].DisplayName != "R2 10MB" || body.Download[1].DisplayOrder != 20 {
		t.Fatalf("download catalog = %+v, want r2_1mb then r2_10mb", body.Download)
	}
}

func TestChartsOverview(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/charts/overview?range=24h&bucket=5m&max_points=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body chartsOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Range != "24h" || body.Bucket != "5m" || body.BucketSeconds != 300 || body.MaxPoints != 10 {
		t.Fatalf("overview metadata = %+v, want range/bucket/max_points", body)
	}
	if len(body.Ping) != 1 || len(body.HTTP) != 2 || len(body.Download) != 1 || len(body.ServiceGroups) != 2 {
		t.Fatalf("overview counts = ping:%d http:%d download:%d groups:%d", len(body.Ping), len(body.HTTP), len(body.Download), len(body.ServiceGroups))
	}
	if body.HTTP[0].Name != "youtube_home" || body.HTTP[0].DisplayName != "YouTube Home" || body.HTTP[0].DisplayOrder != 10 || body.HTTP[1].Name != "steam_store" || body.HTTP[1].DisplayName != "Steam Store" || body.HTTP[1].DisplayOrder != 90 {
		t.Fatalf("overview HTTP = %+v, want display order", body.HTTP)
	}
}

func TestMonitoringThresholds(t *testing.T) {
	state := collector.NewState()
	thresholds := config.DefaultMonitoringThresholds()
	thresholds.HTTP.TotalMs = config.Threshold{Warning: 2500, Critical: 4500}
	handler := New(state, "test").WithMonitoringThresholds(thresholds).Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/thresholds", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		HTTP struct {
			TotalMs config.Threshold `json:"total_ms"`
		} `json:"http"`
		Download map[string]config.Threshold `json:"download"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.HTTP.TotalMs.Warning != 2500 || body.HTTP.TotalMs.Critical != 4500 || body.Download["r2_1mb_mbps"].Warning != 5 {
		t.Fatalf("threshold body = %+v, want configured thresholds", body)
	}
}

func TestCapabilities(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Service    string               `json:"service"`
		APIVersion string               `json:"api_version"`
		Features   map[string]bool      `json:"features"`
		Chart      chartSupportResponse `json:"chart"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Service != "netwatch" || body.APIVersion != apiVersion || !body.Features["download"] || !body.Features["download_series"] || !body.Features["charts_download"] || len(body.Chart.Ranges) == 0 || body.Chart.MaxPoints["default"] != defaultMaxPoints {
		t.Fatalf("capabilities = %+v, want service/api/chart support", body)
	}
}

func TestServicesSeriesRejectsGroupAndName(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/services/series?group=youtube&name=youtube_home", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestServicesSummary(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/services/summary?range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Groups []serviceSummaryResponse `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(body.Groups))
	}
	youtube := body.Groups[1]
	if youtube.Group != "youtube" || youtube.DisplayName != "Youtube" || youtube.SampleCount != 2 || youtube.OKCount != 2 || youtube.OKRate != 100 || youtube.AvgTotalMs != 600 || youtube.MaxTotalMs != 800 {
		t.Fatalf("youtube summary = %+v, want count 2 ok 100 avg 600 max 800", youtube)
	}
	steam := body.Groups[0]
	if steam.Group != "steam" || steam.SampleCount != 1 || steam.OKRate != 0 || steam.TimeoutCount != 1 || steam.ErrorCount != 1 {
		t.Fatalf("steam summary = %+v, want timeout/error summary", steam)
	}
}

func TestMonitoringStatusIgnoresHTTPFailure(t *testing.T) {
	state := collector.NewState()
	ok := true
	failed := false
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: time.Now(), Type: "http", Group: "steam", Category: "service", Name: "steam_store", URL: "https://store.steampowered.com/", OK: &failed, TotalMs: floatPtr(1), Error: "context deadline exceeded"},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Alert || body.Level != "ok" || body.StatusID != "ok" || body.PrimaryReason != nil || len(body.Reasons) != 0 {
		t.Fatalf("body = %+v, want HTTP service failure excluded from core monitoring", body)
	}
}

func TestMonitoringStatusWarnsOnDownloadSlow(t *testing.T) {
	state := collector.NewState()
	ok := true
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: time.Now(), Type: "download", Name: "r2_1mb", URL: "https://example.com/netwatch-1mb.bin", OK: &ok, Mbps: floatPtr(3.2)},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !body.Alert || body.Level != "warning" || body.Message != "download r2_1mb 3.2Mbps" || body.PrimaryReason == nil || body.PrimaryReason.Code != "download_slow" {
		t.Fatalf("body = %+v, want download warning", body)
	}
}

func TestMonitoringStatusIgnoresProviderStatus(t *testing.T) {
	state := collector.NewState()
	failed := false
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "status_page", Name: "github_status", OK: &failed, Level: "critical", Indicator: "major"},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Level != "ok" || body.Alert || body.PrimaryReason != nil || len(body.Reasons) != 0 {
		t.Fatalf("body = %+v, want provider status excluded from core monitoring", body)
	}
}

func TestMonitoringStatusIgnoresUnknownProviderStatus(t *testing.T) {
	state := collector.NewState()
	failed := false
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "status_page", Name: "github_status", OK: &failed, Level: "unknown", Error: "request timeout"},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Level != "ok" || body.Alert {
		t.Fatalf("body = %+v, want unknown provider ignored by monitoring status", body)
	}
}

func TestMonitoringStatusOK(t *testing.T) {
	state := collector.NewState()
	ok := true
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(1)},
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(30)},
		{Timestamp: time.Now(), Type: "dns", Name: "lookup", OK: &ok, DurationMs: floatPtr(20)},
		{Timestamp: time.Now(), Type: "http", Group: "youtube", Category: "service", Name: "youtube_home", OK: &ok, TotalMs: floatPtr(1000)},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Alert || body.Source != "netwatch" || body.StatusID != "ok" || body.Level != "ok" || body.Title != "NET OK" || body.Message != "core network probes within thresholds" || body.PrimaryReason != nil || len(body.Reasons) != 0 {
		t.Fatalf("body = %+v, want ok", body)
	}
}

func TestMonitoringStatusCriticalThresholds(t *testing.T) {
	state := collector.NewState()
	ok := true
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(220)},
	})
	handler := New(state, "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body monitoringStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !body.Alert || body.Level != "critical" || body.Title != "NET CRITICAL" || body.PrimaryReason == nil || body.PrimaryReason.Code != "external_rtt_high" {
		t.Fatalf("body = %+v, want critical alert", body)
	}
}

func newTestHandler() http.Handler {
	state := collector.NewState()
	ok := true
	failed := false
	status := http.StatusOK
	expectedBytes := int64(1048576)
	downloadedBytes := int64(1048576)
	old := time.Now().Add(-2 * time.Hour)
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "ping", Name: "cloudflare_dns", DisplayOrder: 30, Target: "1.1.1.1", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: now, Type: "dns", Name: "lookup", Hostname: "www.google.com", OK: &ok, DurationMs: floatPtr(12.3)},
		{Timestamp: now, Type: "http", Name: "home", URL: "https://example.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(45.6)},
		{Timestamp: old, Type: "http", Group: "youtube", Category: "service", Name: "youtube_home", DisplayOrder: 10, URL: "https://www.youtube.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(400), DNSMs: floatPtr(10), ConnectMs: floatPtr(20), TLSMs: floatPtr(30), TTFBMs: floatPtr(40)},
		{Timestamp: now, Type: "http", Group: "youtube", Category: "service", Name: "youtube_home", DisplayOrder: 10, URL: "https://www.youtube.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(800), DNSMs: floatPtr(20), ConnectMs: floatPtr(30), TLSMs: floatPtr(40), TTFBMs: floatPtr(50)},
		{Timestamp: now, Type: "http", Group: "steam", Category: "service", Name: "steam_store", DisplayOrder: 90, URL: "https://store.steampowered.com/", Method: "GET", OK: &failed, TotalMs: floatPtr(0), Error: "timeout"},
		{Timestamp: now, Type: "http", Group: "pcgame", Category: "game", Name: "sf6_buckler_info", DisplayOrder: 110, URL: "https://www.streetfighter.com/6/buckler/en/information/all/1", Method: "GET", OK: &failed, HTTPStatus: intPtr(http.StatusForbidden), TotalMs: floatPtr(0)},
		{Timestamp: now, Type: "download", Name: "r2_1mb", DisplayOrder: 10, URL: "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin", OK: &ok, ExpectedBytes: &expectedBytes, DownloadedBytes: &downloadedBytes, DurationMs: floatPtr(1000), BytesPerSec: floatPtr(1048576), Mbps: floatPtr(8.388608)},
		{Timestamp: now, Type: "status_page", Group: "github", Category: "dev", Name: "github_status", DisplayOrder: 10, OK: &ok, Level: "ok", Indicator: "none", Description: "All Systems Operational", DurationMs: floatPtr(123), Components: []model.StatusPageComponent{{Name: "API Requests", Status: "operational", Level: "ok", Important: true}}},
	})
	targets := []config.TargetConfig{
		{Name: "gateway", Label: "Gateway", DisplayOrder: 10, Type: "ping", Target: "192.168.1.1"},
		{Name: "google_dns", Label: "Google DNS", DisplayOrder: 20, Type: "ping", Target: "8.8.8.8"},
		{Name: "cloudflare_dns", Label: "Cloudflare DNS", DisplayOrder: 30, Type: "ping", Target: "1.1.1.1"},
		{Name: "lookup", Label: "Lookup", Type: "dns", Hostname: "www.google.com"},
		{Name: "youtube_home", Label: "YouTube Home", DisplayOrder: 10, Type: "http", Group: "youtube", Category: "service", URL: "https://www.youtube.com/"},
		{Name: "steam_store", Label: "Steam Store", DisplayOrder: 90, Type: "http", Group: "steam", Category: "service", URL: "https://store.steampowered.com/"},
		{Name: "sf6_buckler_info", Label: "SF6 Buckler Info", DisplayOrder: 110, Type: "http", Group: "pcgame", Category: "game", URL: "https://www.streetfighter.com/6/buckler/en/information/all/1"},
	}
	downloadProbes := []config.DownloadProbeConfig{
		{Name: "r2_1mb", Label: "R2 1MB", DisplayOrder: 10, URL: "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin", ExpectedBytes: 1048576, IntervalSeconds: 600, TimeoutSeconds: 20, Enabled: true},
		{Name: "r2_10mb", Label: "R2 10MB", DisplayOrder: 20, URL: "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-10mb.bin", ExpectedBytes: 10485760, IntervalSeconds: 3600, TimeoutSeconds: 60, Enabled: true},
	}
	statusPages := []config.StatusPageConfig{
		{Name: "github_status", Label: "GitHub Status", DisplayOrder: 10, Type: "status_page", Provider: "statuspage", Group: "github", Category: "dev", URL: "https://www.githubstatus.com/api/v2/summary.json"},
	}
	return New(state, "test", targets).WithDownloadProbes(downloadProbes).WithStatusPages(statusPages).Routes()
}

func assertSampleCount(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Samples []model.Sample `json:"samples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(body.Samples) != want {
		t.Fatalf("len(samples) = %d, want %d", len(body.Samples), want)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
