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

func TestMeasureHTTPUsesTargetMetadataAndTimeout(t *testing.T) {
	httpProbe := &fakeHTTPProbe{}
	cfg := config.Default()
	target := config.TargetConfig{
		Name:           "youtube_home",
		Type:           "http",
		Group:          "youtube",
		Category:       "service",
		URL:            "https://www.youtube.com/",
		TimeoutSeconds: 3,
	}
	collector := New(cfg, nil, nil, httpProbe, nil, NewState())

	before := time.Now()
	sample := collector.measureHTTP(context.Background(), target)
	remaining := time.Until(httpProbe.deadline)

	if sample.Group != "youtube" || sample.Category != "service" {
		t.Fatalf("sample = %+v, want group/category metadata", sample)
	}
	if sample.TotalMs == nil || *sample.TotalMs != 12.3 {
		t.Fatalf("TotalMs = %v, want 12.3", sample.TotalMs)
	}
	if httpProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want target timeout around 3s", remaining)
	}
}
