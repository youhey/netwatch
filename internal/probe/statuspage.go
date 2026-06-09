package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

const StatusPageUserAgent = "netwatch-status-page/0.2"

type StatusPageResult struct {
	OK                    bool
	Level                 string
	Indicator             string
	Description           string
	DurationMs            float64
	Components            []model.StatusPageComponent
	Incidents             []model.StatusPageIncident
	ScheduledMaintenances []model.StatusPageScheduledMaint
}

type StatusPage struct {
	Client       *http.Client
	MaxBodyBytes int64
}

func NewStatusPage() StatusPage {
	return StatusPage{
		Client:       &http.Client{},
		MaxBodyBytes: 1024 * 1024,
	}
}

func (p StatusPage) Get(ctx context.Context, url string, importantComponents []string) (StatusPageResult, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{}
	}
	maxBodyBytes := p.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 1024 * 1024
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return StatusPageResult{Level: "unknown"}, err
	}
	req.Header.Set("User-Agent", StatusPageUserAgent)

	start := time.Now()
	resp, err := client.Do(req)
	result := StatusPageResult{Level: "unknown", DurationMs: durationMs(start, time.Now())}
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	result.DurationMs = durationMs(start, time.Now())
	if err != nil {
		return result, err
	}
	if int64(len(body)) > maxBodyBytes {
		return result, fmt.Errorf("response body exceeded %d bytes", maxBodyBytes)
	}

	parsed, err := ParseStatusPageSummary(body, importantComponents)
	parsed.DurationMs = result.DurationMs
	if err != nil {
		return parsed, err
	}
	return parsed, nil
}

func ParseStatusPageSummary(body []byte, importantComponents []string) (StatusPageResult, error) {
	var summary statusPageSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return StatusPageResult{Level: "unknown"}, err
	}

	important := make(map[string]struct{}, len(importantComponents))
	for _, name := range importantComponents {
		important[strings.TrimSpace(name)] = struct{}{}
	}

	result := StatusPageResult{
		Indicator:   summary.Status.Indicator,
		Description: summary.Status.Description,
		Level:       statusIndicatorLevel(summary.Status.Indicator),
	}

	importantCritical := false
	importantWarning := false
	for _, component := range summary.Components {
		_, isImportant := important[component.Name]
		level := componentStatusLevel(component.Status)
		result.Components = append(result.Components, model.StatusPageComponent{
			Name:      component.Name,
			Status:    component.Status,
			Level:     level,
			Important: isImportant,
		})
		if !isImportant {
			continue
		}
		switch level {
		case "critical":
			importantCritical = true
		case "warning":
			importantWarning = true
		}
	}

	for _, incident := range summary.Incidents {
		result.Incidents = append(result.Incidents, model.StatusPageIncident{
			ID:        incident.ID,
			Name:      incident.Name,
			Status:    incident.Status,
			Impact:    incident.Impact,
			UpdatedAt: parseOptionalTime(incident.UpdatedAt),
			Shortlink: incident.Shortlink,
		})
	}
	for _, maintenance := range summary.ScheduledMaintenances {
		result.ScheduledMaintenances = append(result.ScheduledMaintenances, model.StatusPageScheduledMaint{
			ID:             maintenance.ID,
			Name:           maintenance.Name,
			Status:         maintenance.Status,
			Impact:         maintenance.Impact,
			ScheduledFor:   parseOptionalTime(maintenance.ScheduledFor),
			ScheduledUntil: parseOptionalTime(maintenance.ScheduledUntil),
			UpdatedAt:      parseOptionalTime(maintenance.UpdatedAt),
			Shortlink:      maintenance.Shortlink,
		})
	}

	result.Level = providerStatusLevel(result.Level, importantCritical, importantWarning, result.Incidents, result.ScheduledMaintenances)
	result.OK = result.Level == "ok"
	return result, nil
}

func statusIndicatorLevel(indicator string) string {
	switch strings.ToLower(strings.TrimSpace(indicator)) {
	case "none":
		return "ok"
	case "minor":
		return "warning"
	case "major", "critical":
		return "critical"
	default:
		return "unknown"
	}
}

func componentStatusLevel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "operational":
		return "ok"
	case "degraded_performance", "partial_outage", "under_maintenance":
		return "warning"
	case "major_outage":
		return "critical"
	default:
		return "unknown"
	}
}

func providerStatusLevel(indicatorLevel string, importantCritical, importantWarning bool, incidents []model.StatusPageIncident, maintenances []model.StatusPageScheduledMaint) string {
	if indicatorLevel == "critical" || importantCritical {
		return "critical"
	}
	if indicatorLevel == "warning" || importantWarning || len(incidents) > 0 || hasActiveMaintenance(maintenances) {
		return "warning"
	}
	if indicatorLevel == "unknown" {
		return "unknown"
	}
	return "ok"
}

func hasActiveMaintenance(maintenances []model.StatusPageScheduledMaint) bool {
	for _, maintenance := range maintenances {
		status := strings.ToLower(strings.TrimSpace(maintenance.Status))
		impact := strings.ToLower(strings.TrimSpace(maintenance.Impact))
		if status == "in_progress" || status == "verifying" {
			return true
		}
		if status != "" && status != "completed" && impact != "" && impact != "none" {
			return true
		}
	}
	return false
}

func parseOptionalTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &t
}

type statusPageSummary struct {
	Status struct {
		Description string `json:"description"`
		Indicator   string `json:"indicator"`
	} `json:"status"`
	Components []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"components"`
	Incidents []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		Impact    string `json:"impact"`
		UpdatedAt string `json:"updated_at"`
		Shortlink string `json:"shortlink"`
	} `json:"incidents"`
	ScheduledMaintenances []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Status         string `json:"status"`
		Impact         string `json:"impact"`
		ScheduledFor   string `json:"scheduled_for"`
		ScheduledUntil string `json:"scheduled_until"`
		UpdatedAt      string `json:"updated_at"`
		Shortlink      string `json:"shortlink"`
	} `json:"scheduled_maintenances"`
}
