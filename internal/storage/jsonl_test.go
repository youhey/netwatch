package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/model"
)

func TestLoadMixedJSONLSkipsInvalidLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.jsonl")
	now := time.Now().UTC()
	content := `{"ts":"` + now.Format(time.RFC3339Nano) + `","type":"ping","name":"cloudflare_dns","target":"1.1.1.1","ok":true}
not-json
{"ts":"` + now.Format(time.RFC3339Nano) + `","type":"dns","name":"lookup","hostname":"www.google.com","ok":true,"duration_ms":12.3}
{"ts":"` + now.Format(time.RFC3339Nano) + `","type":"http","name":"home","url":"https://example.com/","method":"GET","ok":true,"http_status":200,"total_ms":45.6}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	samples, err := NewJSONL(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("len(samples) = %d, want 3", len(samples))
	}

	state := collector.NewState()
	state.Load(samples)
	if len(state.LatestByType("ping")) != 1 || len(state.LatestByType("dns")) != 1 || len(state.LatestByType("http")) != 1 {
		t.Fatalf("latest counts = ping:%d dns:%d http:%d, want 1 each", len(state.LatestByType("ping")), len(state.LatestByType("dns")), len(state.LatestByType("http")))
	}
}

func TestAppendMixedSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.jsonl")
	ok := true
	duration := 10.5
	status := 200
	total := 20.5

	jsonl := NewJSONL(path)
	samples := []model.Sample{
		{Timestamp: time.Now().UTC(), Type: "ping", Name: "ping", Target: "1.1.1.1", OK: &ok},
		{Timestamp: time.Now().UTC(), Type: "dns", Name: "dns", Hostname: "example.com", OK: &ok, DurationMs: &duration},
		{Timestamp: time.Now().UTC(), Type: "http", Name: "http", URL: "https://example.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: &total},
	}
	for _, sample := range samples {
		if err := jsonl.Append(sample); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	loaded, err := jsonl.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("len(loaded) = %d, want 3", len(loaded))
	}
}
