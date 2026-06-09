package model

import "time"

type Sample struct {
	Timestamp    time.Time `json:"ts"`
	Kind         string    `json:"kind,omitempty"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	Group        string    `json:"group,omitempty"`
	Category     string    `json:"category,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	DisplayOrder int       `json:"display_order,omitempty"`
	OK           *bool     `json:"ok,omitempty"`
	Error        string    `json:"error,omitempty"`

	Target      string   `json:"target,omitempty"`
	Sent        int      `json:"sent,omitempty"`
	Received    int      `json:"received,omitempty"`
	LossPercent *float64 `json:"loss_percent,omitempty"`
	RTTMinMs    *float64 `json:"rtt_min_ms,omitempty"`
	RTTAvgMs    *float64 `json:"rtt_avg_ms,omitempty"`
	RTTMaxMs    *float64 `json:"rtt_max_ms,omitempty"`

	Hostname    string   `json:"hostname,omitempty"`
	DurationMs  *float64 `json:"duration_ms,omitempty"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`

	URL               string   `json:"url,omitempty"`
	Method            string   `json:"method,omitempty"`
	HTTPStatus        *int     `json:"http_status,omitempty"`
	DNSMs             *float64 `json:"dns_ms,omitempty"`
	ConnectMs         *float64 `json:"connect_ms,omitempty"`
	TLSMs             *float64 `json:"tls_ms,omitempty"`
	TTFBMs            *float64 `json:"ttfb_ms,omitempty"`
	TotalMs           *float64 `json:"total_ms,omitempty"`
	RemoteAddr        string   `json:"remote_addr,omitempty"`
	ContentLength     *int64   `json:"content_length,omitempty"`
	ContentLengthRead *int64   `json:"content_length_read,omitempty"`
	BodyTruncated     *bool    `json:"body_truncated,omitempty"`

	ExpectedBytes   *int64   `json:"expected_bytes,omitempty"`
	DownloadedBytes *int64   `json:"downloaded_bytes,omitempty"`
	BytesPerSec     *float64 `json:"bytes_per_sec,omitempty"`
	Mbps            *float64 `json:"mbps,omitempty"`

	RetryState           string     `json:"retry_state,omitempty"`
	RetryAttempt         *int       `json:"retry_attempt,omitempty"`
	RecoverySuccessCount *int       `json:"recovery_success_count,omitempty"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`

	Provider              string                     `json:"provider,omitempty"`
	Level                 string                     `json:"level,omitempty"`
	Indicator             string                     `json:"indicator,omitempty"`
	Description           string                     `json:"description,omitempty"`
	Components            []StatusPageComponent      `json:"components,omitempty"`
	Incidents             []StatusPageIncident       `json:"incidents,omitempty"`
	ScheduledMaintenances []StatusPageScheduledMaint `json:"scheduled_maintenances,omitempty"`
}

type StatusPageComponent struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Level     string `json:"level"`
	Important bool   `json:"important"`
}

type StatusPageIncident struct {
	ID        string     `json:"id,omitempty"`
	Name      string     `json:"name,omitempty"`
	Status    string     `json:"status,omitempty"`
	Impact    string     `json:"impact,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Shortlink string     `json:"shortlink,omitempty"`
}

type StatusPageScheduledMaint struct {
	ID             string     `json:"id,omitempty"`
	Name           string     `json:"name,omitempty"`
	Status         string     `json:"status,omitempty"`
	Impact         string     `json:"impact,omitempty"`
	ScheduledFor   *time.Time `json:"scheduled_for,omitempty"`
	ScheduledUntil *time.Time `json:"scheduled_until,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	Shortlink      string     `json:"shortlink,omitempty"`
}
