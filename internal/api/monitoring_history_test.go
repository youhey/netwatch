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

func TestParseMonitoringHistoryRange(t *testing.T) {
	duration, err := parseMonitoringHistoryRange("2h")
	if err != nil {
		t.Fatalf("parseMonitoringHistoryRange() error = %v", err)
	}
	if duration != 2*time.Hour {
		t.Fatalf("duration = %v, want 2h", duration)
	}
}

func TestParseMonitoringHistoryBucket(t *testing.T) {
	bucket, err := parseMonitoringHistoryBucket("5m")
	if err != nil {
		t.Fatalf("parseMonitoringHistoryBucket() error = %v", err)
	}
	if bucket != 5*time.Minute {
		t.Fatalf("bucket = %v, want 5m", bucket)
	}
}

func TestMonitoringStatusHistoryRejectsInvalidRange(t *testing.T) {
	handler := New(collector.NewState(), "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status/history?range=48h&bucket=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
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
	if body.Error.Code != "invalid_range" || body.Error.Param != "range" {
		t.Fatalf("body = %+v, want invalid range error", body)
	}
}

func TestMonitoringStatusHistoryRejectsInvalidBucket(t *testing.T) {
	handler := New(collector.NewState(), "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status/history?range=24h&bucket=10m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
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
	if body.Error.Code != "invalid_bucket" || body.Error.Param != "bucket" {
		t.Fatalf("body = %+v, want invalid bucket error", body)
	}
}

func TestBuildMonitoringStatusHistoryBucketsAndSummary(t *testing.T) {
	ok := true
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.Local)
	bucket := time.Hour
	duration := 24 * time.Hour
	end := nextBucketBoundary(now, bucket)
	start := end.Add(-duration)
	samples := []model.Sample{
		{Timestamp: start.Add(time.Hour + 10*time.Minute), Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(1)},
		{Timestamp: start.Add(2*time.Hour + 10*time.Minute), Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
		{Timestamp: start.Add(3*time.Hour + 10*time.Minute), Type: "download", Name: "r2_1mb", OK: &ok, Mbps: floatPtr(3.2)},
		{Timestamp: start.Add(3*time.Hour + 20*time.Minute), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(220)},
	}

	body := buildMonitoringStatusHistory(samples, config.DefaultMonitoringThresholds(), "24h", "1h", duration, bucket, start, end, now)

	if body.Source != "netwatch" || body.Range != "24h" || body.Bucket != "1h" || body.BucketSeconds != 3600 {
		t.Fatalf("metadata = %+v, want status history metadata", body)
	}
	if body.GeneratedAt.IsZero() || body.ActualRangeStart.IsZero() || body.ActualRangeEnd.IsZero() {
		t.Fatalf("metadata timestamps = %+v, want generated/range timestamps", body)
	}
	if len(body.Points) != 24 {
		t.Fatalf("len(points) = %d, want 24", len(body.Points))
	}
	if body.Points[0].Level != "unknown" || body.Points[0].SampleCount != 0 {
		t.Fatalf("points[0] = %+v, want unknown empty bucket", body.Points[0])
	}
	if body.Points[1].Level != "ok" || body.Points[1].OKCount != 1 {
		t.Fatalf("points[1] = %+v, want ok bucket", body.Points[1])
	}
	if body.Points[2].Level != "warning" || !body.Points[2].Alert || body.Points[2].WarningCount != 1 {
		t.Fatalf("points[2] = %+v, want warning bucket", body.Points[2])
	}
	if body.Points[3].Level != "critical" || !body.Points[3].Alert || body.Points[3].CriticalCount != 1 || body.Points[3].WarningCount != 1 {
		t.Fatalf("points[3] = %+v, want critical priority bucket", body.Points[3])
	}
	for i := 1; i < len(body.Points); i++ {
		if !body.Points[i-1].BucketStart.Before(body.Points[i].BucketStart) {
			t.Fatalf("points are not chronological at %d: %+v then %+v", i, body.Points[i-1], body.Points[i])
		}
	}
	if body.Summary.OKCount != 1 || body.Summary.WarningCount != 1 || body.Summary.CriticalCount != 1 || body.Summary.UnknownCount != 21 {
		t.Fatalf("summary = %+v, want point-level summary", body.Summary)
	}
}

func TestBuildMonitoringStatusHistoryTwoHourFiveMinuteBuckets(t *testing.T) {
	ok := true
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.Local)
	bucket := 5 * time.Minute
	duration := 2 * time.Hour
	end := nextBucketBoundary(now, bucket)
	start := end.Add(-duration)
	samples := []model.Sample{
		{Timestamp: start.Add(5*time.Minute + time.Minute), Type: "ping", Name: "gateway", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(1)},
	}

	body := buildMonitoringStatusHistory(samples, config.DefaultMonitoringThresholds(), "2h", "5m", duration, bucket, start, end, now)

	if body.Range != "2h" || body.Bucket != "5m" || body.BucketSeconds != 300 {
		t.Fatalf("metadata = %+v, want 2h/5m", body)
	}
	if len(body.Points) != 24 {
		t.Fatalf("len(points) = %d, want 24", len(body.Points))
	}
	if body.Points[0].Level != "unknown" || body.Points[0].SampleCount != 0 {
		t.Fatalf("points[0] = %+v, want unknown empty bucket", body.Points[0])
	}
	if body.Points[1].Level != "ok" || body.Points[1].OKCount != 1 {
		t.Fatalf("points[1] = %+v, want ok bucket", body.Points[1])
	}
}

func TestBuildMonitoringStatusHistoryIgnoresHTTPServiceIssues(t *testing.T) {
	ok := true
	failed := false
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.Local)
	bucket := time.Hour
	duration := 24 * time.Hour
	end := nextBucketBoundary(now, bucket)
	start := end.Add(-duration)
	samples := []model.Sample{
		{Timestamp: start.Add(time.Hour + 10*time.Minute), Type: "http", Group: "github", Name: "github_home", OK: &failed, Error: "unexpected status 503"},
		{Timestamp: start.Add(time.Hour + 20*time.Minute), Type: "http", Group: "chatgpt", Name: "chatgpt_home", OK: &ok, TotalMs: floatPtr(6000)},
	}

	body := buildMonitoringStatusHistory(samples, config.DefaultMonitoringThresholds(), "24h", "1h", duration, bucket, start, end, now)

	if body.Points[1].Level != "unknown" || body.Points[1].Alert || body.Points[1].SampleCount != 0 || body.Points[1].WarningCount != 0 {
		t.Fatalf("points[1] = %+v, want HTTP services excluded from status history", body.Points[1])
	}
	if body.Summary.UnknownCount != 24 {
		t.Fatalf("summary = %+v, want HTTP-only history ignored", body.Summary)
	}
}

func TestMonitoringStatusHistoryEndpointTwoHourFiveMinuteBuckets(t *testing.T) {
	handler := New(collector.NewState(), "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/status/history?range=2h&bucket=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body monitoringStatusHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if body.Range != "2h" || body.Bucket != "5m" || len(body.Points) != 24 || body.Points[0].Level != "unknown" {
		t.Fatalf("body = %+v, want 2h/5m unknown history", body)
	}
}

func TestBuildMonitoringStatusHistoryKeepsTwentyFourHourOneHourBuckets(t *testing.T) {
	body := buildMonitoringStatusHistory(nil, config.DefaultMonitoringThresholds(), "24h", "1h", 24*time.Hour, time.Hour, time.Now().Add(-24*time.Hour), time.Now(), time.Now())

	if body.Range != "24h" || body.Bucket != "1h" || len(body.Points) != 24 {
		t.Fatalf("history = %+v, want existing 24h/1h support", body)
	}
}

func TestMonitoringStatusHistoryCapabilities(t *testing.T) {
	handler := New(collector.NewState(), "test").Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body struct {
		Features map[string]bool `json:"features"`
		History  struct {
			Ranges  []string `json:"ranges"`
			Buckets []string `json:"buckets"`
		} `json:"monitoring_status_history"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !body.Features["monitoring_status_history"] || !body.Features["monitoring_status_history_2h_5m"] || len(body.History.Ranges) == 0 || len(body.History.Buckets) == 0 {
		t.Fatalf("capabilities = %+v, want monitoring status history support", body)
	}
	if !containsString(body.History.Ranges, "2h") || !containsString(body.History.Buckets, "5m") {
		t.Fatalf("capabilities history = %+v, want 2h/5m support", body.History)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
