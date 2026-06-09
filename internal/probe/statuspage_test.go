package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseStatusPageIndicatorLevels(t *testing.T) {
	tests := []struct {
		indicator string
		wantLevel string
		wantOK    bool
	}{
		{indicator: "none", wantLevel: "ok", wantOK: true},
		{indicator: "minor", wantLevel: "warning"},
		{indicator: "major", wantLevel: "critical"},
		{indicator: "critical", wantLevel: "critical"},
		{indicator: "", wantLevel: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.indicator, func(t *testing.T) {
			body := `{"status":{"description":"status","indicator":"` + tt.indicator + `"},"components":[],"incidents":[],"scheduled_maintenances":[]}`
			result, err := ParseStatusPageSummary([]byte(body), nil)
			if err != nil {
				t.Fatalf("ParseStatusPageSummary() error = %v", err)
			}
			if result.Level != tt.wantLevel || result.OK != tt.wantOK {
				t.Fatalf("result = %+v, want level=%s ok=%v", result, tt.wantLevel, tt.wantOK)
			}
		})
	}
}

func TestParseStatusPageComponentLevels(t *testing.T) {
	body := `{
  "status": {"description": "All Systems Operational", "indicator": "none"},
  "components": [
    {"name": "API Requests", "status": "operational"},
    {"name": "Git Operations", "status": "degraded_performance"},
    {"name": "Webhooks", "status": "partial_outage"},
    {"name": "Actions", "status": "major_outage"},
    {"name": "Packages", "status": "under_maintenance"}
  ],
  "incidents": [],
  "scheduled_maintenances": []
}`
	result, err := ParseStatusPageSummary([]byte(body), []string{"API Requests"})
	if err != nil {
		t.Fatalf("ParseStatusPageSummary() error = %v", err)
	}
	if result.Level != "ok" {
		t.Fatalf("Level = %s, want ok because non-important components are ignored", result.Level)
	}
	levels := map[string]string{}
	for _, component := range result.Components {
		levels[component.Name] = component.Level
	}
	if levels["API Requests"] != "ok" || levels["Git Operations"] != "warning" || levels["Webhooks"] != "warning" || levels["Actions"] != "critical" || levels["Packages"] != "warning" {
		t.Fatalf("component levels = %+v, want mapped levels", levels)
	}
}

func TestParseStatusPageImportantComponentAffectsLevel(t *testing.T) {
	body := `{
  "status": {"description": "All Systems Operational", "indicator": "none"},
  "components": [
    {"name": "API Requests", "status": "major_outage"}
  ],
  "incidents": [],
  "scheduled_maintenances": []
}`
	result, err := ParseStatusPageSummary([]byte(body), []string{"API Requests"})
	if err != nil {
		t.Fatalf("ParseStatusPageSummary() error = %v", err)
	}
	if result.Level != "critical" || result.OK {
		t.Fatalf("result = %+v, want critical", result)
	}
}

func TestParseStatusPageIncidentsAndMaintenancesWarn(t *testing.T) {
	body := `{
  "status": {"description": "All Systems Operational", "indicator": "none"},
  "components": [],
  "incidents": [
    {"id":"i1","name":"Incident","status":"investigating","impact":"minor","updated_at":"2026-06-09T14:02:19Z","shortlink":"https://stspg.io/i1"}
  ],
  "scheduled_maintenances": [
    {"id":"m1","name":"Maintenance","status":"scheduled","impact":"none","scheduled_for":"2026-06-10T14:00:00Z","scheduled_until":"2026-06-10T15:00:00Z"}
  ]
}`
	result, err := ParseStatusPageSummary([]byte(body), nil)
	if err != nil {
		t.Fatalf("ParseStatusPageSummary() error = %v", err)
	}
	if result.Level != "warning" || len(result.Incidents) != 1 || result.Incidents[0].UpdatedAt == nil || len(result.ScheduledMaintenances) != 1 {
		t.Fatalf("result = %+v, want warning with metadata", result)
	}
}

func TestParseStatusPageActiveMaintenanceWarns(t *testing.T) {
	body := `{
  "status": {"description": "All Systems Operational", "indicator": "none"},
  "components": [],
  "incidents": [],
  "scheduled_maintenances": [
    {"id":"m1","name":"Maintenance","status":"in_progress","impact":"none"}
  ]
}`
	result, err := ParseStatusPageSummary([]byte(body), nil)
	if err != nil {
		t.Fatalf("ParseStatusPageSummary() error = %v", err)
	}
	if result.Level != "warning" {
		t.Fatalf("Level = %s, want warning for active maintenance", result.Level)
	}
}

func TestParseStatusPageInvalidJSON(t *testing.T) {
	result, err := ParseStatusPageSummary([]byte(`{`), nil)
	if err == nil {
		t.Fatal("ParseStatusPageSummary() error = nil, want error")
	}
	if result.Level != "unknown" || result.OK {
		t.Fatalf("result = %+v, want unknown failure", result)
	}
}

func TestStatusPageGetTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":{"indicator":"none"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	result, err := NewStatusPage().Get(ctx, server.URL, nil)
	if err == nil {
		t.Fatal("Get() error = nil, want timeout")
	}
	if result.Level != "unknown" || result.OK {
		t.Fatalf("result = %+v, want unknown failure", result)
	}
}

func TestStatusPageGetOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != StatusPageUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, StatusPageUserAgent)
		}
		_, _ = w.Write([]byte(`{"status":{"description":"All Systems Operational","indicator":"none"},"components":[],"incidents":[],"scheduled_maintenances":[]}`))
	}))
	defer server.Close()

	result, err := NewStatusPage().Get(context.Background(), server.URL, nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK || result.Level != "ok" || result.DurationMs < 0 {
		t.Fatalf("result = %+v, want ok with duration", result)
	}
}

func TestStatusPageGetBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 20)))
	}))
	defer server.Close()

	result, err := StatusPage{MaxBodyBytes: 5}.Get(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("Get() error = nil, want body limit error")
	}
	if result.Level != "unknown" {
		t.Fatalf("result = %+v, want unknown", result)
	}
}
