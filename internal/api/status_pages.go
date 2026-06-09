package api

import (
	"time"

	"github.com/youhey/netwatch/internal/model"
)

type statusPagesLatestBody struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	Providers   []statusPageProviderResponse `json:"providers"`
}

type statusPageProviderResponse struct {
	Name                  string                           `json:"name"`
	Label                 string                           `json:"label"`
	Group                 string                           `json:"group,omitempty"`
	Category              string                           `json:"category,omitempty"`
	Level                 string                           `json:"level"`
	OK                    bool                             `json:"ok"`
	Indicator             string                           `json:"indicator,omitempty"`
	Description           string                           `json:"description,omitempty"`
	DurationMs            *float64                         `json:"duration_ms,omitempty"`
	Components            []model.StatusPageComponent      `json:"components"`
	Incidents             []model.StatusPageIncident       `json:"incidents"`
	ScheduledMaintenances []model.StatusPageScheduledMaint `json:"scheduled_maintenances"`
	Error                 string                           `json:"error,omitempty"`
	MeasuredAt            time.Time                        `json:"measured_at"`
}

type providerStatusSummaryResponse struct {
	Level     string `json:"level"`
	OK        bool   `json:"ok"`
	Providers int    `json:"providers"`
	Warning   int    `json:"warning"`
	Critical  int    `json:"critical"`
	Unknown   int    `json:"unknown"`
}

type compactProviderStatusResponse struct {
	Level      string                          `json:"level"`
	Alert      bool                            `json:"alert"`
	IssueCount int                             `json:"issue_count"`
	Providers  []compactProviderStatusProvider `json:"providers"`
}

type compactProviderStatusProvider struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Level       string `json:"level"`
	Description string `json:"description,omitempty"`
}

func statusPagesLatestResponse(samples []model.Sample, generatedAt time.Time) statusPagesLatestBody {
	providers := make([]statusPageProviderResponse, 0, len(samples))
	for _, sample := range samples {
		providers = append(providers, statusPageProvider(sample))
	}
	return statusPagesLatestBody{
		GeneratedAt: generatedAt,
		Providers:   providers,
	}
}

func statusPageProvider(sample model.Sample) statusPageProviderResponse {
	level := sample.Level
	if level == "" {
		level = "unknown"
	}
	ok := sample.OK != nil && *sample.OK
	components := sample.Components
	if components == nil {
		components = []model.StatusPageComponent{}
	}
	incidents := sample.Incidents
	if incidents == nil {
		incidents = []model.StatusPageIncident{}
	}
	maintenances := sample.ScheduledMaintenances
	if maintenances == nil {
		maintenances = []model.StatusPageScheduledMaint{}
	}
	return statusPageProviderResponse{
		Name:                  sample.Name,
		Label:                 sample.DisplayName,
		Group:                 sample.Group,
		Category:              sample.Category,
		Level:                 level,
		OK:                    ok,
		Indicator:             sample.Indicator,
		Description:           sample.Description,
		DurationMs:            sample.DurationMs,
		Components:            components,
		Incidents:             incidents,
		ScheduledMaintenances: maintenances,
		Error:                 sample.Error,
		MeasuredAt:            sample.Timestamp,
	}
}

func providerStatusSummary(samples []model.Sample) providerStatusSummaryResponse {
	summary := providerStatusSummaryResponse{Level: "ok", OK: true, Providers: len(samples)}
	for _, sample := range samples {
		switch statusPageLevel(sample) {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		case "unknown":
			summary.Unknown++
		}
	}
	summary.Level = aggregateProviderStatusLevel(summary.Warning, summary.Critical, summary.Unknown)
	summary.OK = summary.Level == "ok"
	return summary
}

func compactProviderStatus(samples []model.Sample) compactProviderStatusResponse {
	summary := providerStatusSummary(samples)
	providers := make([]compactProviderStatusProvider, 0, len(samples))
	for _, sample := range samples {
		providers = append(providers, compactProviderStatusProvider{
			Name:        sample.Name,
			Label:       sample.DisplayName,
			Level:       statusPageLevel(sample),
			Description: sample.Description,
		})
	}
	return compactProviderStatusResponse{
		Level:      summary.Level,
		Alert:      summary.Level == "warning" || summary.Level == "critical",
		IssueCount: summary.Warning + summary.Critical,
		Providers:  providers,
	}
}

func aggregateProviderStatusLevel(warning, critical, unknown int) string {
	if critical > 0 {
		return "critical"
	}
	if warning > 0 {
		return "warning"
	}
	if unknown > 0 {
		return "unknown"
	}
	return "ok"
}

func statusPageLevel(sample model.Sample) string {
	if sample.Level == "" {
		return "unknown"
	}
	return sample.Level
}
