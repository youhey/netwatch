package api

import (
	"fmt"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

const (
	compactHistoryRange  = "2h"
	compactHistoryBucket = "5m"
	compactHistoryPoints = 24
)

type monitoringCompactSupportResponse struct {
	HistoryRange  string `json:"history_range"`
	HistoryBucket string `json:"history_bucket"`
	HistoryPoints int    `json:"history_points"`
}

type monitoringCompactResponse struct {
	Source         string                        `json:"source"`
	GeneratedAt    time.Time                     `json:"generated_at"`
	Level          string                        `json:"level"`
	Label          string                        `json:"label"`
	Alert          bool                          `json:"alert"`
	Title          string                        `json:"title"`
	Message        string                        `json:"message"`
	IssueCount     int                           `json:"issue_count"`
	PrimaryReason  *monitoringCompactReason      `json:"primary_reason"`
	History        monitoringCompactHistory      `json:"history"`
	ProviderStatus compactProviderStatusResponse `json:"provider_status"`
}

type monitoringCompactReason struct {
	Code   string  `json:"code"`
	Level  string  `json:"level"`
	Target string  `json:"target"`
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
}

type monitoringCompactHistory struct {
	Range         string                          `json:"range"`
	Bucket        string                          `json:"bucket"`
	BucketSeconds int                             `json:"bucket_seconds"`
	Points        []monitoringCompactHistoryPoint `json:"points"`
}

type monitoringCompactHistoryPoint struct {
	Level string `json:"level"`
	Alert bool   `json:"alert"`
}

func monitoringCompactSupport() monitoringCompactSupportResponse {
	return monitoringCompactSupportResponse{
		HistoryRange:  compactHistoryRange,
		HistoryBucket: compactHistoryBucket,
		HistoryPoints: compactHistoryPoints,
	}
}

func buildMonitoringCompact(status monitoringStatusResponse, history monitoringStatusHistoryResponse, generatedAt time.Time, providerSamples ...[]model.Sample) monitoringCompactResponse {
	var statusPageSamples []model.Sample
	if len(providerSamples) > 0 {
		statusPageSamples = providerSamples[0]
	}
	return monitoringCompactResponse{
		Source:         "netwatch",
		GeneratedAt:    generatedAt,
		Level:          status.Level,
		Label:          compactLabel(status.Level),
		Alert:          status.Alert,
		Title:          compactTitle(status.Level),
		Message:        compactMessage(status.Level, status.PrimaryReason),
		IssueCount:     len(status.Reasons),
		PrimaryReason:  compactReason(status.PrimaryReason),
		History:        compactHistory(history),
		ProviderStatus: compactProviderStatus(statusPageSamples),
	}
}

func compactHistory(history monitoringStatusHistoryResponse) monitoringCompactHistory {
	points := make([]monitoringCompactHistoryPoint, 0, len(history.Points))
	for _, point := range history.Points {
		points = append(points, monitoringCompactHistoryPoint{
			Level: point.Level,
			Alert: point.Alert,
		})
	}
	return monitoringCompactHistory{
		Range:         history.Range,
		Bucket:        history.Bucket,
		BucketSeconds: history.BucketSeconds,
		Points:        points,
	}
}

func compactReason(reason *monitoringReason) *monitoringCompactReason {
	if reason == nil {
		return nil
	}
	return &monitoringCompactReason{
		Code:   reason.Code,
		Level:  reason.Level,
		Target: reason.Target,
		Metric: reason.Metric,
		Value:  reason.Value,
	}
}

func compactLabel(level string) string {
	switch level {
	case "warning":
		return "WARN"
	case "critical":
		return "CRIT"
	case "unknown":
		return "UNK"
	default:
		return "NET OK"
	}
}

func compactTitle(level string) string {
	switch level {
	case "warning":
		return "Network degradation detected"
	case "critical":
		return "Critical network issue detected"
	case "unknown":
		return "Monitoring status unavailable"
	default:
		return "All systems operational"
	}
}

func compactMessage(level string, reason *monitoringReason) string {
	if level == "ok" {
		return "All probes are healthy."
	}
	if level == "unknown" || reason == nil {
		return "Netwatch cannot determine current network health."
	}
	switch reason.Code {
	case "gateway_loss", "packet_loss":
		return fmt.Sprintf("Packet loss is above the %s threshold on %s.", reason.Level, reason.Target)
	case "gateway_rtt_high", "external_rtt_high":
		return fmt.Sprintf("Round-trip latency is above the %s threshold on %s.", reason.Level, reason.Target)
	case "dns_failure":
		return fmt.Sprintf("DNS probe is failing on %s.", reason.Target)
	case "dns_slow":
		return fmt.Sprintf("DNS latency is above the %s threshold on %s.", reason.Level, reason.Target)
	case "http_timeout":
		return fmt.Sprintf("HTTP probe is timing out on %s.", reason.Target)
	case "http_slow":
		return fmt.Sprintf("HTTP latency is above the %s threshold on %s.", reason.Level, reason.Target)
	case "service_failure":
		return fmt.Sprintf("Service probe is failing on %s.", reason.Target)
	case "service_group_degraded":
		return fmt.Sprintf("Service group health is below the %s threshold on %s.", reason.Level, reason.Target)
	case "download_failure":
		return fmt.Sprintf("Download probe is failing on %s.", reason.Target)
	case "download_slow":
		return fmt.Sprintf("Download throughput is below the %s threshold on %s.", reason.Level, reason.Target)
	case "provider_status":
		return fmt.Sprintf("Provider status is reporting an issue on %s.", reason.Target)
	default:
		return reasonMessage(*reason)
	}
}
