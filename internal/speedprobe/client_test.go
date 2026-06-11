package speedprobe

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientLatestDecodesResponse(t *testing.T) {
	var userAgent string
	client := Client{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		userAgent = req.Header.Get("User-Agent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(bytes.NewBufferString(`{
  "generated_at": "2026-06-12T00:00:00Z",
  "service": "netwatch-speedprobe",
  "version": "test",
  "observer": {
    "hostname": "scum",
    "interface": "eth0",
    "link_speed": "1000Mb/s",
    "duplex": "full",
    "operstate": "up"
  },
  "probes": [
    {
      "name": "r2_10mb",
      "label": "R2 10MB",
      "status": "ok",
      "running": false,
      "manual_only": false,
      "enabled": true,
      "url": "https://example.com/10mb.bin",
      "expected_bytes": 10485760,
      "downloaded_bytes": 10485760,
      "duration_ms": 1200.5,
      "mbps": 69.9,
      "measured_at": "2026-06-12T00:00:01Z",
      "last_run_id": "run-1"
    }
  ]
}`)),
			Header: make(http.Header),
		}, nil
	})}}

	latest, err := client.Latest(context.Background(), "http://speedprobe.local/api/v1/speed/latest", time.Second)
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if userAgent != UserAgent {
		t.Fatalf("User-Agent = %q, want %q", userAgent, UserAgent)
	}
	if latest.Service != "netwatch-speedprobe" || latest.Observer.Hostname != "scum" || len(latest.Probes) != 1 {
		t.Fatalf("latest = %+v, want decoded response", latest)
	}
	probe := latest.Probes[0]
	if probe.Name != "r2_10mb" || probe.Mbps == nil || *probe.Mbps != 69.9 || probe.MeasuredAt == nil || probe.LastRunID != "run-1" {
		t.Fatalf("probe = %+v, want decoded probe metrics", probe)
	}
}

func TestClientLatestRejectsNon2xx(t *testing.T) {
	client := Client{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
		}, nil
	})}}

	_, err := client.Latest(context.Background(), "http://speedprobe.local/api/v1/speed/latest", time.Second)
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("Latest() error = %v, want HTTP 503 error", err)
	}
}
