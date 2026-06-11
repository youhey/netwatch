package speedprobe

import "time"

type LatestResponse struct {
	GeneratedAt time.Time `json:"generated_at"`
	Service     string    `json:"service"`
	Version     string    `json:"version"`
	Observer    Observer  `json:"observer"`
	Probes      []Probe   `json:"probes"`
}

type Observer struct {
	Hostname  string `json:"hostname"`
	Interface string `json:"interface"`
	LinkSpeed string `json:"link_speed"`
	Duplex    string `json:"duplex"`
	Operstate string `json:"operstate"`
}

type Probe struct {
	Name            string     `json:"name"`
	Label           string     `json:"label"`
	Status          string     `json:"status"`
	Running         bool       `json:"running"`
	ManualOnly      bool       `json:"manual_only"`
	Enabled         bool       `json:"enabled"`
	URL             string     `json:"url"`
	HTTPStatusCode  *int       `json:"http_status_code"`
	ExpectedBytes   *int64     `json:"expected_bytes"`
	DownloadedBytes *int64     `json:"downloaded_bytes"`
	DurationMs      *float64   `json:"duration_ms"`
	Mbps            *float64   `json:"mbps"`
	Error           *string    `json:"error"`
	MeasuredAt      *time.Time `json:"measured_at"`
	LastRunID       string     `json:"last_run_id"`
}
