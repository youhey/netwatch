package api

import (
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

func TestParseBucket(t *testing.T) {
	tests := map[string]time.Duration{
		"1m":  time.Minute,
		"5m":  5 * time.Minute,
		"15m": 15 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  time.Hour,
		"6h":  6 * time.Hour,
		"1d":  24 * time.Hour,
	}
	for value, want := range tests {
		got, err := parseBucket(value)
		if err != nil {
			t.Fatalf("parseBucket(%q) error = %v", value, err)
		}
		if got != want {
			t.Fatalf("parseBucket(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestParseBucketUnsupported(t *testing.T) {
	if _, err := parseBucket("2m"); err == nil {
		t.Fatal("parseBucket() error = nil, want error")
	}
}

func TestParseMaxPoints(t *testing.T) {
	got, err := parseMaxPoints("")
	if err != nil {
		t.Fatalf("parseMaxPoints() error = %v", err)
	}
	if got != defaultMaxPoints {
		t.Fatalf("parseMaxPoints() = %d, want %d", got, defaultMaxPoints)
	}

	got, err = parseMaxPoints("300")
	if err != nil {
		t.Fatalf("parseMaxPoints() error = %v", err)
	}
	if got != 300 {
		t.Fatalf("parseMaxPoints() = %d, want 300", got)
	}
}

func TestParseMaxPointsOutOfRange(t *testing.T) {
	if _, err := parseMaxPoints("9"); err == nil {
		t.Fatal("parseMaxPoints() error = nil, want error")
	}
	if _, err := parseMaxPoints("2001"); err == nil {
		t.Fatal("parseMaxPoints() error = nil, want error")
	}
}

func TestAggregatePing(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	samples := []model.Sample{
		{Timestamp: base, Sent: 10, Received: 9, RTTAvgMs: floatPtr(10), RTTMinMs: floatPtr(8), RTTMaxMs: floatPtr(15)},
		{Timestamp: base.Add(time.Minute), Sent: 10, Received: 10, RTTAvgMs: floatPtr(20), RTTMinMs: floatPtr(9), RTTMaxMs: floatPtr(25)},
	}

	points := aggregatePing(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 2 || point.AvgMs == nil || *point.AvgMs != 15 || point.MinMs == nil || *point.MinMs != 8 || point.MaxMs == nil || *point.MaxMs != 25 {
		t.Fatalf("point = %+v, want ping aggregate", point)
	}
	if point.LossPercent == nil || *point.LossPercent != 5 {
		t.Fatalf("LossPercent = %v, want 5", point.LossPercent)
	}
}

func TestAggregateDNS(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	ok := true
	failed := false
	samples := []model.Sample{
		{Timestamp: base, OK: &ok, DurationMs: floatPtr(10)},
		{Timestamp: base.Add(time.Minute), OK: &ok, DurationMs: floatPtr(20)},
		{Timestamp: base.Add(2 * time.Minute), OK: &failed},
	}

	points := aggregateDNS(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 3 || point.FailureCount != 1 || point.AvgMs == nil || *point.AvgMs != 15 || point.MinMs == nil || *point.MinMs != 10 || point.MaxMs == nil || *point.MaxMs != 20 {
		t.Fatalf("point = %+v, want dns aggregate", point)
	}
}

func TestAggregateHTTP(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	ok := true
	failed := false
	samples := []model.Sample{
		{Timestamp: base, OK: &ok, TotalMs: floatPtr(100), TTFBMs: floatPtr(20)},
		{Timestamp: base.Add(time.Minute), OK: &ok, TotalMs: floatPtr(300), TTFBMs: floatPtr(40)},
		{Timestamp: base.Add(2 * time.Minute), OK: &failed, TotalMs: floatPtr(0), Error: "timeout"},
	}

	points := aggregateHTTP(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 3 || point.FailureCount != 1 || point.TimeoutCount != 1 {
		t.Fatalf("point = %+v, want counts", point)
	}
	if point.AvgTotalMs == nil || *point.AvgTotalMs != 200 || point.MaxTotalMs == nil || *point.MaxTotalMs != 300 || point.AvgTTFBMs == nil || *point.AvgTTFBMs != 30 {
		t.Fatalf("point = %+v, want http aggregate", point)
	}
}

func TestAggregateServices(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	ok := true
	failed := false
	samples := []model.Sample{
		{Timestamp: base, Name: "riot_status", OK: &ok, TotalMs: floatPtr(100)},
		{Timestamp: base.Add(time.Minute), Name: "ea_status", OK: &failed, Error: "timeout"},
	}

	points := aggregateServices(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 2 || point.FailureCount != 1 || point.OKRate == nil || *point.OKRate != 50 || point.AvgTotalMs == nil || *point.AvgTotalMs != 100 {
		t.Fatalf("point = %+v, want service aggregate", point)
	}
}

func TestAggregateDownload(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	ok := true
	failed := false
	samples := []model.Sample{
		{Timestamp: base, OK: &ok, Mbps: floatPtr(10)},
		{Timestamp: base.Add(time.Minute), OK: &ok, Mbps: floatPtr(20)},
		{Timestamp: base.Add(2 * time.Minute), OK: &failed, Error: "timeout"},
	}

	points := aggregateDownload(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 3 || point.FailureCount != 1 || point.TimeoutCount != 1 {
		t.Fatalf("point = %+v, want download counts", point)
	}
	if point.AvgMbps == nil || *point.AvgMbps != 15 || point.MinMbps == nil || *point.MinMbps != 10 || point.MaxMbps == nil || *point.MaxMbps != 20 {
		t.Fatalf("point = %+v, want download Mbps aggregate", point)
	}
}

func TestAggregateDownloadFailureOnlyOmitsMbps(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	failed := false
	samples := []model.Sample{
		{Timestamp: base, OK: &failed, Error: "http status 404"},
	}

	points := aggregateDownload(samples, 5*time.Minute)
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	point := points[0]
	if point.SampleCount != 1 || point.FailureCount != 1 || point.AvgMbps != nil || point.MinMbps != nil || point.MaxMbps != nil {
		t.Fatalf("point = %+v, want failure-only download aggregate", point)
	}
}

func TestThinPoints(t *testing.T) {
	points := make([]chartPoint, 20)
	for i := range points {
		points[i] = chartPoint{Timestamp: time.Unix(int64(i), 0), SampleCount: 1}
	}
	got := thinPoints(points, 10)
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want 10", len(got))
	}
}
