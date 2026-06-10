package api

import (
	"sort"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

type compactServiceHealthResponse struct {
	Level      string                      `json:"level"`
	Alert      bool                        `json:"alert"`
	IssueCount int                         `json:"issue_count"`
	Summary    []compactServiceHealthGroup `json:"summary"`
	Issues     []compactServiceHealthIssue `json:"issues"`
}

type compactServiceHealthGroup struct {
	Group string `json:"group"`
	Label string `json:"label"`
	Level string `json:"level"`
	OK    int    `json:"ok"`
	Total int    `json:"total"`

	displayOrder int
}

type compactServiceHealthIssue struct {
	Name           string    `json:"name"`
	Label          string    `json:"label"`
	Group          string    `json:"group,omitempty"`
	Category       string    `json:"category,omitempty"`
	Level          string    `json:"level"`
	Reason         string    `json:"reason"`
	HTTPStatusCode *int      `json:"http_status_code,omitempty"`
	DurationMs     *float64  `json:"duration_ms,omitempty"`
	MeasuredAt     time.Time `json:"measured_at"`

	displayOrder int
}

type serviceHealthSummaryResponse struct {
	Level      string `json:"level"`
	Alert      bool   `json:"alert"`
	IssueCount int    `json:"issue_count"`
	Groups     int    `json:"groups"`
	Services   int    `json:"services"`
}

func compactServiceHealth(samples []model.Sample, thresholds config.MonitoringThresholds) compactServiceHealthResponse {
	groups := make(map[string]*compactServiceHealthGroup)
	issues := make([]compactServiceHealthIssue, 0)
	serviceFailures := serviceFailureCounts(samples)
	overallLevel := "ok"

	for _, sample := range samples {
		if !isServiceHealthSample(sample) {
			continue
		}
		level, reason := serviceHealthLevelAndReason(sample, thresholds, serviceFailures)
		if serviceHealthRank(level) > serviceHealthRank(overallLevel) {
			overallLevel = level
		}

		group := groups[sample.Group]
		if group == nil {
			group = &compactServiceHealthGroup{
				Group:        sample.Group,
				Label:        serviceHealthGroupLabel(sample.Group, sample.Category),
				Level:        "ok",
				displayOrder: displayOrderRank(sample.DisplayOrder),
			}
			groups[sample.Group] = group
		}
		if rank := displayOrderRank(sample.DisplayOrder); rank < group.displayOrder {
			group.displayOrder = rank
		}
		group.Total++
		if level == "ok" {
			group.OK++
		}
		if serviceHealthRank(level) > serviceHealthRank(group.Level) {
			group.Level = level
		}

		if level == "ok" {
			continue
		}
		issues = append(issues, compactServiceHealthIssue{
			Name:           sample.Name,
			Label:          sample.DisplayName,
			Group:          sample.Group,
			Category:       sample.Category,
			Level:          level,
			Reason:         reason,
			HTTPStatusCode: sample.HTTPStatus,
			DurationMs:     serviceHealthDuration(sample),
			MeasuredAt:     sample.Timestamp,
			displayOrder:   displayOrderRank(sample.DisplayOrder),
		})
	}

	summary := make([]compactServiceHealthGroup, 0, len(groups))
	for _, group := range groups {
		summary = append(summary, *group)
	}
	sort.SliceStable(summary, func(i, j int) bool {
		if summary[i].displayOrder != summary[j].displayOrder {
			return summary[i].displayOrder < summary[j].displayOrder
		}
		return summary[i].Group < summary[j].Group
	})
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].displayOrder != issues[j].displayOrder {
			return issues[i].displayOrder < issues[j].displayOrder
		}
		return issues[i].Name < issues[j].Name
	})

	for i := range summary {
		summary[i].displayOrder = 0
	}
	for i := range issues {
		issues[i].displayOrder = 0
	}

	return compactServiceHealthResponse{
		Level:      overallLevel,
		Alert:      false,
		IssueCount: len(issues),
		Summary:    summary,
		Issues:     issues,
	}
}

func serviceHealthSummary(samples []model.Sample, thresholds config.MonitoringThresholds) serviceHealthSummaryResponse {
	health := compactServiceHealth(samples, thresholds)
	services := 0
	for _, group := range health.Summary {
		services += group.Total
	}
	return serviceHealthSummaryResponse{
		Level:      health.Level,
		Alert:      health.Alert,
		IssueCount: health.IssueCount,
		Groups:     len(health.Summary),
		Services:   services,
	}
}

func isServiceHealthSample(sample model.Sample) bool {
	return sample.Type == "http" && sample.Group != "" && !isIgnoredServiceTarget(sample)
}

func serviceHealthLevelAndReason(sample model.Sample, thresholds config.MonitoringThresholds, serviceFailures map[string]int) (string, string) {
	if sample.OK == nil {
		return "unknown", "unknown_error"
	}
	reasons := httpReasons(sample, thresholds.HTTP, serviceFailures)
	if len(reasons) == 0 {
		return "ok", ""
	}
	level := levelForReasons(reasons)
	if level == "ok" {
		return "ok", ""
	}
	if !*sample.OK || sample.Error != "" {
		return level, serviceHealthFailureReason(sample)
	}
	return level, "slow_response"
}

func serviceHealthFailureReason(sample model.Sample) string {
	errorText := strings.ToLower(strings.TrimSpace(sample.Error))
	if isTimeoutError(errorText) {
		return "timeout"
	}
	if strings.Contains(errorText, "dns") || strings.Contains(errorText, "lookup") || strings.Contains(errorText, "no such host") {
		return "dns_error"
	}
	if strings.Contains(errorText, "tls") || strings.Contains(errorText, "certificate") {
		return "tls_error"
	}
	if strings.Contains(errorText, "connect") || strings.Contains(errorText, "connection refused") || strings.Contains(errorText, "network is unreachable") {
		return "tcp_error"
	}
	if sample.HTTPStatus != nil || strings.Contains(errorText, "unexpected status") || strings.Contains(errorText, "status code") {
		return "unexpected_status"
	}
	return "unknown_error"
}

func serviceHealthDuration(sample model.Sample) *float64 {
	if sample.TotalMs != nil {
		return sample.TotalMs
	}
	return sample.DurationMs
}

func serviceHealthRank(level string) int {
	switch level {
	case "critical":
		return 4
	case "warning":
		return 3
	case "unknown":
		return 2
	case "ok":
		return 1
	default:
		return 0
	}
}

func serviceHealthGroupLabel(group, category string) string {
	groupLabels := map[string]string{
		"github":     "Dev Core",
		"openai":     "AI",
		"laravel":    "Deploy",
		"docker":     "Container",
		"baseline":   "Baseline",
		"youtube":    "Entertainment",
		"netflix":    "Entertainment",
		"steam":      "Entertainment",
		"psn":        "Game",
		"pcgame":     "Game",
		"pc-game":    "Game",
		"pc_game":    "Game",
		"aws":        "Cloud",
		"azure":      "Cloud",
		"cloudflare": "Cloud",
		"slack":      "Service",
	}
	if label, ok := groupLabels[group]; ok {
		return label
	}
	categoryLabels := map[string]string{
		"dev":       "Dev Core",
		"ai":        "AI",
		"cloud":     "Cloud",
		"container": "Container",
		"service":   "Service",
		"game":      "Game",
		"baseline":  "Baseline",
	}
	if label, ok := categoryLabels[category]; ok {
		return label
	}
	return labelForName(group)
}
