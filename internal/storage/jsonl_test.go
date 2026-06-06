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
{"ts":"` + now.Format(time.RFC3339Nano) + `","type":"http","group":"youtube","category":"service","name":"youtube_home","url":"https://www.youtube.com/","method":"GET","ok":true,"http_status":200,"total_ms":312.4}
{"ts":"` + now.Format(time.RFC3339Nano) + `","type":"download","name":"r2_1mb","url":"https://example.com/netwatch-1mb.bin","expected_bytes":1048576,"downloaded_bytes":1048576,"duration_ms":1000,"bytes_per_sec":1048576,"mbps":8.388608,"ok":true}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	samples, err := NewJSONL(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(samples) != 5 {
		t.Fatalf("len(samples) = %d, want 5", len(samples))
	}

	state := collector.NewState()
	state.Load(samples)
	if len(state.LatestByType("ping")) != 1 || len(state.LatestByType("dns")) != 1 || len(state.LatestByType("http")) != 2 || len(state.LatestByType("download")) != 1 {
		t.Fatalf("latest counts = ping:%d dns:%d http:%d download:%d, want ping:1 dns:1 http:2 download:1", len(state.LatestByType("ping")), len(state.LatestByType("dns")), len(state.LatestByType("http")), len(state.LatestByType("download")))
	}
	if services := state.LatestServices(); len(services) != 1 || services[0].Group != "youtube" || services[0].Category != "service" {
		t.Fatalf("LatestServices() = %+v, want youtube service", services)
	}
}

func TestAppendMixedSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.jsonl")
	ok := true
	duration := 10.5
	status := 200
	total := 20.5
	expectedBytes := int64(1048576)
	downloadedBytes := int64(1048576)
	mbps := 8.388608

	jsonl := NewJSONL(path)
	samples := []model.Sample{
		{Timestamp: time.Now().UTC(), Type: "ping", Name: "ping", Target: "1.1.1.1", OK: &ok},
		{Timestamp: time.Now().UTC(), Type: "dns", Name: "dns", Hostname: "example.com", OK: &ok, DurationMs: &duration},
		{Timestamp: time.Now().UTC(), Type: "http", Group: "youtube", Category: "service", Name: "http", URL: "https://example.com/", Method: "GET", OK: &ok, HTTPStatus: &status, TotalMs: &total},
		{Timestamp: time.Now().UTC(), Type: "download", Name: "r2_1mb", URL: "https://example.com/netwatch-1mb.bin", ExpectedBytes: &expectedBytes, DownloadedBytes: &downloadedBytes, DurationMs: &duration, Mbps: &mbps, OK: &ok},
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
	if len(loaded) != 4 {
		t.Fatalf("len(loaded) = %d, want 4", len(loaded))
	}
	if loaded[2].Group != "youtube" || loaded[2].Category != "service" {
		t.Fatalf("loaded[2] = %+v, want group/category restored", loaded[2])
	}
	if loaded[3].DownloadedBytes == nil || *loaded[3].DownloadedBytes != downloadedBytes {
		t.Fatalf("loaded[3] = %+v, want download restored", loaded[3])
	}
}

func TestRotatingJSONLAppendUsesDailyFile(t *testing.T) {
	dir := t.TempDir()
	jsonl := NewRotatingJSONL(dir, "samples-%Y-%m-%d.jsonl", 14)
	ok := true

	if err := jsonl.Append(model.Sample{Timestamp: time.Now().UTC(), Type: "http", Name: "home", OK: &ok}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	path := filepath.Join(dir, "samples-"+time.Now().Format("2006-01-02")+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
}

func TestRotatingJSONLLoadMultipleDaysAndCleanup(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "samples-"+time.Now().AddDate(0, 0, -20).Format("2006-01-02")+".jsonl")
	recentPath := filepath.Join(dir, "samples-"+time.Now().AddDate(0, 0, -1).Format("2006-01-02")+".jsonl")
	todayPath := filepath.Join(dir, "samples-"+time.Now().Format("2006-01-02")+".jsonl")

	for path, name := range map[string]string{
		oldPath:    "old",
		recentPath: "recent",
		todayPath:  "today",
	} {
		content := `{"ts":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","type":"http","group":"youtube","name":"` + name + `","ok":true}` + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	samples, err := NewRotatingJSONL(dir, "samples-%Y-%m-%d.jsonl", 14).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2", len(samples))
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or unexpected error: %v", err)
	}
}

func TestSingleDataPathStillWorks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.jsonl")
	jsonl := NewJSONL(path)
	ok := true

	if err := jsonl.Append(model.Sample{Timestamp: time.Now().UTC(), Type: "ping", Name: "ping", OK: &ok}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	samples, err := jsonl.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(samples))
	}
}
