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
	if len(body["ping"]) != 1 || len(body["dns"]) != 1 || len(body["http"]) != 3 {
		t.Fatalf("counts = ping:%d dns:%d http:%d, want ping:1 dns:1 http:3", len(body["ping"]), len(body["dns"]), len(body["http"]))
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

	assertSampleCount(t, rec, 3)
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
	if body.Services[0].Group != "steam" || body.Services[0].Status != "warning" {
		t.Fatalf("services[0] = %+v, want steam warning", body.Services[0])
	}
	if body.Services[1].Group != "youtube" || body.Services[1].Status != "ok" {
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
	if youtube.Group != "youtube" || youtube.SampleCount != 2 || youtube.OKRate != 100 || youtube.AvgTotalMs != 600 || youtube.MaxTotalMs != 800 {
		t.Fatalf("youtube summary = %+v, want count 2 ok 100 avg 600 max 800", youtube)
	}
	steam := body.Groups[0]
	if steam.Group != "steam" || steam.SampleCount != 1 || steam.OKRate != 0 || steam.TimeoutCount != 1 || steam.ErrorCount != 1 {
		t.Fatalf("steam summary = %+v, want timeout/error summary", steam)
	}
}

func TestMonitoringStatusWarnsOnHTTPFailure(t *testing.T) {
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
	if !body.Alert || body.Level != "warning" {
		t.Fatalf("body = %+v, want warning alert", body)
	}
	if body.Message != "steam timeout" {
		t.Fatalf("Message = %q, want steam timeout", body.Message)
	}
}

func newTestHandler() http.Handler {
	state := collector.NewState()
	ok := true
	failed := false
	status := http.StatusOK
	old := time.Now().Add(-2 * time.Hour)
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "ping", Name: "cloudflare_dns", Target: "1.1.1.1", OK: &ok, LossPercent: floatPtr(0), RTTAvgMs: floatPtr(10)},
		{Timestamp: now, Type: "dns", Name: "lookup", Hostname: "www.google.com", OK: &ok, DurationMs: floatPtr(12.3)},
		{Timestamp: now, Type: "http", Name: "home", URL: "https://example.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(45.6)},
		{Timestamp: old, Type: "http", Group: "youtube", Category: "service", Name: "youtube_home", URL: "https://www.youtube.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(400)},
		{Timestamp: now, Type: "http", Group: "youtube", Category: "service", Name: "youtube_home", URL: "https://www.youtube.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: floatPtr(800)},
		{Timestamp: now, Type: "http", Group: "steam", Category: "service", Name: "steam_store", URL: "https://store.steampowered.com/", Method: "GET", OK: &failed, TotalMs: floatPtr(0), Error: "timeout"},
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
