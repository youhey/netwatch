package collector

import (
	"context"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/probe"
)

type fakeHTTPProbe struct {
	deadline time.Time
}

type fakeDownloadProbe struct {
	deadline time.Time
}

func (p *fakeHTTPProbe) Get(ctx context.Context, url string) (probe.HTTPResult, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		p.deadline = deadline
	}
	status := 200
	return probe.HTTPResult{
		OK:         true,
		HTTPStatus: &status,
		TotalMs:    12.3,
	}, nil
}

func (p *fakeDownloadProbe) Get(ctx context.Context, url string, expectedBytes int64) (probe.DownloadResult, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		p.deadline = deadline
	}
	return probe.DownloadResult{
		OK:              true,
		DownloadedBytes: expectedBytes,
		DurationMs:      1000,
		BytesPerSec:     float64(expectedBytes),
		Mbps:            float64(expectedBytes) * 8 / 1_000_000,
	}, nil
}

func TestMeasureHTTPUsesTargetMetadataAndTimeout(t *testing.T) {
	httpProbe := &fakeHTTPProbe{}
	cfg := config.Default()
	target := config.TargetConfig{
		Name:           "youtube_home",
		Type:           "http",
		Group:          "youtube",
		Category:       "service",
		Label:          "YouTube Home",
		URL:            "https://www.youtube.com/",
		TimeoutSeconds: 3,
		DisplayOrder:   10,
	}
	collector := New(cfg, nil, nil, httpProbe, nil, nil, NewState())

	before := time.Now()
	sample := collector.measureHTTP(context.Background(), target)
	remaining := time.Until(httpProbe.deadline)

	if sample.Group != "youtube" || sample.Category != "service" || sample.DisplayName != "YouTube Home" || sample.DisplayOrder != 10 {
		t.Fatalf("sample = %+v, want group/category metadata", sample)
	}
	if sample.TotalMs == nil || *sample.TotalMs != 12.3 {
		t.Fatalf("TotalMs = %v, want 12.3", sample.TotalMs)
	}
	if httpProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want target timeout around 3s", remaining)
	}
}

func TestMeasureDownloadUsesProbeTimeoutAndRecordsMetrics(t *testing.T) {
	downloadProbe := &fakeDownloadProbe{}
	cfg := config.Default()
	target := config.DownloadProbeConfig{
		Name:            "r2_1mb",
		URL:             "https://example.com/netwatch-1mb.bin",
		Label:           "R2 1MB",
		ExpectedBytes:   1048576,
		TimeoutSeconds:  3,
		IntervalSeconds: 600,
		Enabled:         true,
		DisplayOrder:    10,
	}
	collector := New(cfg, nil, nil, nil, downloadProbe, nil, NewState())

	before := time.Now()
	sample := collector.measureDownload(context.Background(), target)
	remaining := time.Until(downloadProbe.deadline)

	if sample.Type != "download" || sample.URL != target.URL || sample.DisplayName != "R2 1MB" || sample.DisplayOrder != 10 || sample.ExpectedBytes == nil || *sample.ExpectedBytes != target.ExpectedBytes {
		t.Fatalf("sample = %+v, want download metadata", sample)
	}
	if sample.DownloadedBytes == nil || *sample.DownloadedBytes != target.ExpectedBytes || sample.Mbps == nil || *sample.Mbps <= 0 {
		t.Fatalf("sample = %+v, want download metrics", sample)
	}
	if downloadProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want probe timeout around 3s", remaining)
	}
}
