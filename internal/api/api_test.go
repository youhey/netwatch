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

func TestParseRange(t *testing.T) {
	tests := map[string]int{
		"1h":  1,
		"6h":  6,
		"24h": 24,
		"7d":  24 * 7,
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
	if len(body["ping"]) != 1 || len(body["dns"]) != 1 || len(body["http"]) != 1 {
		t.Fatalf("counts = ping:%d dns:%d http:%d, want 1 each", len(body["ping"]), len(body["dns"]), len(body["http"]))
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

	assertSampleCount(t, rec, 1)
}

func TestMonitoringStatusWarnsOnHTTPFailure(t *testing.T) {
	state := collector.NewState()
	ok := true
	failed := false
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: time.Now(), Type: "http", Name: "home", URL: "https://example.com/", OK: &failed, TotalMs: floatPtr(1), Error: "timeout"},
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
	if !body.Alert || body.Level != "warning" {
		t.Fatalf("body = %+v, want warning alert", body)
	}
}

func newTestHandler() http.Handler {
	state := collector.NewState()
	ok := true
	status := http.StatusOK
	state.Load([]model.Sample{
		{Timestamp: time.Now(), Type: "ping", Name: "cloudflare_dns", Target: "1.1.1.1", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: time.Now(), Type: "dns", Name: "lookup", Hostname: "www.google.com", OK: &ok, DurationMs: floatPtr(12.3)},
		{Timestamp: time.Now(), Type: "http", Name: "home", URL: "https://example.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(45.6)},
	})
	return New(state, "test").Routes()
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
