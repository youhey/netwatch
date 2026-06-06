package collector

import (
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

func TestLatestByTypeUsesDisplayOrder(t *testing.T) {
	state := NewState()
	ok := true
	now := time.Now()
	state.Load([]model.Sample{
		{Timestamp: now, Type: "ping", Name: "cloudflare_dns", DisplayOrder: 30, OK: &ok},
		{Timestamp: now, Type: "ping", Name: "gateway", DisplayOrder: 10, OK: &ok},
		{Timestamp: now, Type: "ping", Name: "google_dns", DisplayOrder: 20, OK: &ok},
		{Timestamp: now, Type: "ping", Name: "unordered", OK: &ok},
	})

	samples := state.LatestByType("ping")
	names := sampleNames(samples)
	want := []string{"gateway", "google_dns", "cloudflare_dns", "unordered"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %+v, want %+v", names, want)
		}
	}
}

func sampleNames(samples []model.Sample) []string {
	names := make([]string, 0, len(samples))
	for _, sample := range samples {
		names = append(names, sample.Name)
	}
	return names
}
