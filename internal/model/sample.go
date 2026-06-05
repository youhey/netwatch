package model

import "time"

type Sample struct {
	Timestamp   time.Time `json:"ts"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	Target      string    `json:"target"`
	Sent        int       `json:"sent"`
	Received    int       `json:"received"`
	LossPercent float64   `json:"loss_percent"`
	RTTMinMs    *float64  `json:"rtt_min_ms,omitempty"`
	RTTAvgMs    *float64  `json:"rtt_avg_ms,omitempty"`
	RTTMaxMs    *float64  `json:"rtt_max_ms,omitempty"`
	Error       string    `json:"error,omitempty"`
}
