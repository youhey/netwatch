package api

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

type monitoringReason struct {
	Code                 string     `json:"code"`
	Level                string     `json:"level"`
	Target               string     `json:"target"`
	Metric               string     `json:"metric"`
	Value                float64    `json:"value"`
	Warning              *float64   `json:"warning,omitempty"`
	Critical             *float64   `json:"critical,omitempty"`
	RetryState           string     `json:"retry_state,omitempty"`
	RetryAttempt         *int       `json:"retry_attempt,omitempty"`
	RecoverySuccessCount *int       `json:"recovery_success_count,omitempty"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`

	badness float64
	index   int
}

func buildMonitoringStatus(samples []model.Sample, thresholds config.MonitoringThresholds, generatedAt time.Time) monitoringStatusResponse {
	if len(samples) == 0 {
		return monitoringStatusResponse{
			Alert:       true,
			Source:      "netwatch",
			StatusID:    "unknown-no_data",
			GeneratedAt: generatedAt,
			Level:       "unknown",
			Title:       titleForLevel("unknown"),
			Message:     "no samples",
			Reasons:     []monitoringReason{},
		}
	}

	reasons := collectMonitoringReasons(samples, thresholds)
	if len(reasons) == 0 {
		return monitoringStatusResponse{
			Alert:       false,
			Source:      "netwatch",
			StatusID:    "ok",
			GeneratedAt: generatedAt,
			Level:       "ok",
			Title:       titleForLevel("ok"),
			Message:     "all probes healthy",
			Reasons:     []monitoringReason{},
		}
	}

	level := levelForReasons(reasons)
	primary := selectPrimaryReason(reasons)
	message := monitoringMessage(primary, reasons)
	statusID := monitoringStatusID(level, primary, reasons)

	return monitoringStatusResponse{
		Alert:         true,
		Source:        "netwatch",
		StatusID:      statusID,
		GeneratedAt:   generatedAt,
		Level:         level,
		Title:         titleForLevel(level),
		Message:       message,
		PrimaryReason: &primary,
		Reasons:       stripReasonInternals(reasons),
	}
}

func monitoringThresholdsResponse(thresholds config.MonitoringThresholds, generatedAt time.Time) map[string]any {
	return map[string]any{
		"generated_at": generatedAt,
		"ping":         thresholds.Ping,
		"dns":          thresholds.DNS,
		"http":         thresholds.HTTP,
		"download":     thresholds.Download,
		"service":      thresholds.Service,
	}
}

func collectMonitoringReasons(samples []model.Sample, thresholds config.MonitoringThresholds) []monitoringReason {
	reasons := make([]monitoringReason, 0)
	serviceFailures := serviceFailureCounts(samples)
	serviceStats := serviceGroupStats(samples)

	for _, sample := range samples {
		if isIgnoredServiceTarget(sample) {
			continue
		}
		switch sample.Type {
		case "ping":
			reasons = append(reasons, pingReasons(sample, thresholds.Ping)...)
		case "dns":
			reasons = append(reasons, dnsReasons(sample, thresholds.DNS)...)
		case "http":
			reasons = append(reasons, httpReasons(sample, thresholds.HTTP, serviceFailures)...)
		case "download":
			reasons = append(reasons, downloadReasons(sample, thresholds.Download)...)
		case "status_page":
			reasons = append(reasons, statusPageReasons(sample)...)
		}
	}

	reasons = append(reasons, serviceGroupReasons(serviceStats, thresholds.Service)...)
	for i := range reasons {
		reasons[i].index = i
	}
	return reasons
}

func pingReasons(sample model.Sample, thresholds config.PingThresholds) []monitoringReason {
	var reasons []monitoringReason
	lossPercent := valueOrZero(sample.LossPercent)
	rttAvgMs := valueOrZero(sample.RTTAvgMs)

	if sample.Name == "gateway" {
		if reason, ok := evaluateHighBad("gateway_loss", "loss_percent", sample.Name, lossPercent, thresholds.GatewayLossPercent, false); ok {
			reasons = append(reasons, reason)
		}
		if sample.RTTAvgMs != nil {
			if reason, ok := evaluateHighBad("gateway_rtt_high", "rtt_avg_ms", sample.Name, rttAvgMs, thresholds.GatewayRTTAvgMs, true); ok {
				reasons = append(reasons, reason)
			}
		}
		return reasons
	}

	if reason, ok := evaluateHighBad("packet_loss", "loss_percent", sample.Name, lossPercent, thresholds.ExternalLossPercent, true); ok {
		reasons = append(reasons, reason)
	}
	if sample.RTTAvgMs != nil {
		if reason, ok := evaluateHighBad("external_rtt_high", "rtt_avg_ms", sample.Name, rttAvgMs, thresholds.ExternalRTTAvgMs, true); ok {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

func dnsReasons(sample model.Sample, thresholds config.DNSThresholds) []monitoringReason {
	if sampleFailed(sample) {
		return []monitoringReason{failureReason("dns_failure", "ok", sample.Name)}
	}
	if sample.DurationMs == nil {
		return nil
	}
	reason, ok := evaluateHighBad("dns_slow", "duration_ms", sample.Name, *sample.DurationMs, thresholds.DurationMs, true)
	if !ok {
		return nil
	}
	return []monitoringReason{reason}
}

func httpReasons(sample model.Sample, thresholds config.HTTPThresholds, serviceFailures map[string]int) []monitoringReason {
	if sampleFailed(sample) {
		if sample.Group != "" {
			level := "warning"
			if serviceFailures[sample.Group] > 1 {
				level = "critical"
			}
			return []monitoringReason{failureReasonWithLevel("service_failure", level, "ok", sample.Name)}
		}
		return []monitoringReason{failureReason("http_timeout", "ok", sample.Name)}
	}
	if sample.TotalMs == nil {
		return nil
	}
	reason, ok := evaluateHighBad("http_slow", "total_ms", sample.Name, *sample.TotalMs, thresholds.TotalMs, true)
	if !ok {
		return nil
	}
	return []monitoringReason{reason}
}

func downloadReasons(sample model.Sample, thresholds map[string]config.Threshold) []monitoringReason {
	if sampleFailed(sample) {
		return []monitoringReason{withDownloadRetryMetadata(failureReason("download_failure", "ok", sample.Name), sample)}
	}
	if sample.Mbps == nil {
		return nil
	}
	threshold, ok := thresholds[sample.Name+"_mbps"]
	if !ok {
		return nil
	}
	reason, ok := evaluateLowBad("download_slow", "mbps", sample.Name, *sample.Mbps, threshold)
	if !ok {
		return nil
	}
	return []monitoringReason{withDownloadRetryMetadata(reason, sample)}
}

func statusPageReasons(sample model.Sample) []monitoringReason {
	switch statusPageLevel(sample) {
	case "critical", "warning":
		return []monitoringReason{{
			Code:    "provider_status",
			Level:   "warning",
			Target:  sample.Name,
			Metric:  "level",
			Value:   1,
			badness: 1,
		}}
	default:
		return nil
	}
}

func serviceGroupReasons(stats map[string]serviceStatusAggregate, thresholds config.ServiceThresholds) []monitoringReason {
	reasons := make([]monitoringReason, 0)
	for group, stat := range stats {
		if stat.sampleCount < 2 {
			continue
		}
		okRate := float64(stat.okCount) / float64(stat.sampleCount) * 100
		reason, ok := evaluateLowBad("service_group_degraded", "ok_rate_percent", group, okRate, thresholds.OKRatePercent)
		if !ok {
			continue
		}
		reasons = append(reasons, reason)
	}
	sort.SliceStable(reasons, func(i, j int) bool {
		return reasons[i].Target < reasons[j].Target
	})
	return reasons
}

func evaluateHighBad(code, metric, target string, value float64, threshold config.Threshold, warningInclusive bool) (monitoringReason, bool) {
	warning := threshold.Warning
	critical := threshold.Critical
	reason := monitoringReason{
		Code:     code,
		Target:   target,
		Metric:   metric,
		Value:    value,
		Warning:  &warning,
		Critical: &critical,
	}
	if value >= threshold.Critical {
		reason.Level = "critical"
		reason.badness = ratio(value, threshold.Critical)
		return reason, true
	}
	if warningInclusive && value >= threshold.Warning || !warningInclusive && value > threshold.Warning {
		reason.Level = "warning"
		reason.badness = ratio(value, threshold.Warning)
		return reason, true
	}
	return monitoringReason{}, false
}

func evaluateLowBad(code, metric, target string, value float64, threshold config.Threshold) (monitoringReason, bool) {
	warning := threshold.Warning
	critical := threshold.Critical
	reason := monitoringReason{
		Code:     code,
		Target:   target,
		Metric:   metric,
		Value:    value,
		Warning:  &warning,
		Critical: &critical,
	}
	if value < threshold.Critical {
		reason.Level = "critical"
		reason.badness = inverseRatio(threshold.Critical, value)
		return reason, true
	}
	if value < threshold.Warning {
		reason.Level = "warning"
		reason.badness = inverseRatio(threshold.Warning, value)
		return reason, true
	}
	return monitoringReason{}, false
}

func failureReason(code, metric, target string) monitoringReason {
	return failureReasonWithLevel(code, "warning", metric, target)
}

func failureReasonWithLevel(code, level, metric, target string) monitoringReason {
	return monitoringReason{
		Code:    code,
		Level:   level,
		Target:  target,
		Metric:  metric,
		Value:   0,
		badness: 1,
	}
}

func withDownloadRetryMetadata(reason monitoringReason, sample model.Sample) monitoringReason {
	reason.RetryState = sample.RetryState
	reason.RetryAttempt = sample.RetryAttempt
	reason.RecoverySuccessCount = sample.RecoverySuccessCount
	reason.NextCheckAt = sample.NextCheckAt
	return reason
}

func sampleFailed(sample model.Sample) bool {
	return sample.Error != "" || sample.OK != nil && !*sample.OK
}

func levelForReasons(reasons []monitoringReason) string {
	level := "ok"
	for _, reason := range reasons {
		if severityRank(reason.Level) > severityRank(level) {
			level = reason.Level
		}
	}
	return level
}

func selectPrimaryReason(reasons []monitoringReason) monitoringReason {
	if len(reasons) == 0 {
		return monitoringReason{}
	}
	primary := reasons[0]
	for _, reason := range reasons[1:] {
		if compareReasonPriority(reason, primary) {
			primary = reason
		}
	}
	return primary
}

func compareReasonPriority(left, right monitoringReason) bool {
	if severityRank(left.Level) != severityRank(right.Level) {
		return severityRank(left.Level) > severityRank(right.Level)
	}
	if reasonPriority(left.Code) != reasonPriority(right.Code) {
		return reasonPriority(left.Code) < reasonPriority(right.Code)
	}
	if left.badness != right.badness {
		return left.badness > right.badness
	}
	return left.index < right.index
}

func reasonPriority(code string) int {
	priorities := map[string]int{
		"gateway_loss":           10,
		"packet_loss":            20,
		"download_slow":          30,
		"download_failure":       40,
		"http_timeout":           50,
		"dns_failure":            60,
		"external_rtt_high":      70,
		"gateway_rtt_high":       80,
		"dns_slow":               90,
		"http_slow":              100,
		"service_failure":        110,
		"service_group_degraded": 120,
		"provider_status":        130,
	}
	if priority, ok := priorities[code]; ok {
		return priority
	}
	return 1000
}

func monitoringMessage(primary monitoringReason, reasons []monitoringReason) string {
	if len(reasons) == 0 {
		return "all probes healthy"
	}
	messages := []string{reasonMessage(primary)}
	for _, reason := range reasons {
		if reason.Code == primary.Code && reason.Target == primary.Target && reason.Metric == primary.Metric {
			continue
		}
		messages = append(messages, reasonMessage(reason))
		break
	}
	return strings.Join(messages, ", ")
}

func reasonMessage(reason monitoringReason) string {
	switch reason.Code {
	case "gateway_loss", "packet_loss":
		return fmt.Sprintf("packet loss %.1f%%", reason.Value)
	case "gateway_rtt_high", "external_rtt_high":
		return fmt.Sprintf("rtt %s %.0fms", reason.Target, reason.Value)
	case "dns_failure":
		return fmt.Sprintf("dns %s failure", reason.Target)
	case "dns_slow":
		return fmt.Sprintf("dns %s %.0fms", reason.Target, reason.Value)
	case "http_timeout":
		return fmt.Sprintf("http %s timeout", reason.Target)
	case "http_slow":
		return fmt.Sprintf("http %s %.0fms", reason.Target, reason.Value)
	case "service_failure":
		return fmt.Sprintf("service %s failure", reason.Target)
	case "service_group_degraded":
		return fmt.Sprintf("service %s %.0f%%", reason.Target, reason.Value)
	case "download_failure":
		return fmt.Sprintf("download %s failure", reason.Target)
	case "download_slow":
		return fmt.Sprintf("download %s %.1fMbps", reason.Target, reason.Value)
	case "provider_status":
		return fmt.Sprintf("provider %s status", reason.Target)
	default:
		return reason.Code
	}
}

func monitoringStatusID(level string, primary monitoringReason, reasons []monitoringReason) string {
	if level == "ok" {
		return "ok"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, reason.Code+":"+reason.Target)
	}
	sort.Strings(parts)
	fingerprint := strings.Join(parts, "|")
	sum := sha1.Sum([]byte(level + "|" + primary.Code + "|" + primary.Target + "|" + fingerprint))
	return fmt.Sprintf("%s-%s-%s-%s", level, primary.Code, primary.Target, hex.EncodeToString(sum[:])[:8])
}

func stripReasonInternals(reasons []monitoringReason) []monitoringReason {
	stripped := make([]monitoringReason, len(reasons))
	for i, reason := range reasons {
		reason.badness = 0
		reason.index = 0
		stripped[i] = reason
	}
	return stripped
}

type serviceStatusAggregate struct {
	sampleCount int
	okCount     int
}

func serviceFailureCounts(samples []model.Sample) map[string]int {
	counts := make(map[string]int)
	for _, sample := range samples {
		if sample.Type != "http" || sample.Group == "" || isIgnoredServiceTarget(sample) {
			continue
		}
		if sampleFailed(sample) {
			counts[sample.Group]++
		}
	}
	return counts
}

func serviceGroupStats(samples []model.Sample) map[string]serviceStatusAggregate {
	stats := make(map[string]serviceStatusAggregate)
	for _, sample := range samples {
		if sample.Type != "http" || sample.Group == "" || isIgnoredServiceTarget(sample) {
			continue
		}
		stat := stats[sample.Group]
		stat.sampleCount++
		if sample.OK != nil && *sample.OK {
			stat.okCount++
		}
		stats[sample.Group] = stat
	}
	return stats
}

func titleForLevel(level string) string {
	switch level {
	case "critical":
		return "NET CRITICAL"
	case "warning":
		return "NET WARNING"
	case "unknown":
		return "NET UNKNOWN"
	default:
		return "NET OK"
	}
}

func ratio(value, threshold float64) float64 {
	if threshold == 0 {
		return 0
	}
	return value / threshold
}

func inverseRatio(threshold, value float64) float64 {
	if value == 0 {
		return math.Inf(1)
	}
	return threshold / value
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
