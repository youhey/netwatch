package model

import "time"

type Sample struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	OK        *bool     `json:"ok,omitempty"`
	Error     string    `json:"error,omitempty"`

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

	URL           string   `json:"url,omitempty"`
	Method        string   `json:"method,omitempty"`
	HTTPStatus    *int     `json:"http_status,omitempty"`
	DNSMs         *float64 `json:"dns_ms,omitempty"`
	ConnectMs     *float64 `json:"connect_ms,omitempty"`
	TLSMs         *float64 `json:"tls_ms,omitempty"`
	TTFBMs        *float64 `json:"ttfb_ms,omitempty"`
	TotalMs       *float64 `json:"total_ms,omitempty"`
	RemoteAddr    string   `json:"remote_addr,omitempty"`
	ContentLength *int64   `json:"content_length,omitempty"`
}
