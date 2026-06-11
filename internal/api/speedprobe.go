package api

import (
	"sort"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

type speedprobeSummaryResponse struct {
	Sources int                      `json:"sources"`
	Probes  int                      `json:"probes"`
	Latest  []speedprobeSummaryProbe `json:"latest"`
}

type speedprobeSummaryProbe struct {
	Source     string    `json:"source"`
	Name       string    `json:"name"`
	Mbps       *float64  `json:"mbps,omitempty"`
	Status     string    `json:"status"`
	MeasuredAt time.Time `json:"measured_at"`
}

type throughputStatusResponse struct {
	Level      string                   `json:"level"`
	Alert      bool                     `json:"alert"`
	IssueCount int                      `json:"issue_count"`
	Sources    []throughputStatusSource `json:"sources"`
}

type throughputStatusSource struct {
	Name     string                    `json:"name"`
	Label    string                    `json:"label"`
	Type     string                    `json:"type"`
	Observer *model.SpeedprobeObserver `json:"observer,omitempty"`
	Level    string                    `json:"level"`
	Probes   []throughputStatusProbe   `json:"probes"`
}

type throughputStatusProbe struct {
	Name            string     `json:"name"`
	Label           string     `json:"label"`
	Level           string     `json:"level"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason"`
	Mbps            *float64   `json:"mbps,omitempty"`
	DurationMs      *float64   `json:"duration_ms,omitempty"`
	ExpectedBytes   *int64     `json:"expected_bytes,omitempty"`
	DownloadedBytes *int64     `json:"downloaded_bytes,omitempty"`
	ManualOnly      *bool      `json:"manual_only,omitempty"`
	MeasuredAt      time.Time  `json:"measured_at"`
	NextCheckAt     *time.Time `json:"next_check_at,omitempty"`
	RetryState      string     `json:"retry_state,omitempty"`
}

type throughputStatusSummaryResponse struct {
	Level      string `json:"level"`
	Alert      bool   `json:"alert"`
	IssueCount int    `json:"issue_count"`
	Sources    int    `json:"sources"`
	Probes     int    `json:"probes"`
}

func speedprobeSummary(samples []model.Sample) speedprobeSummaryResponse {
	sources := make(map[string]struct{})
	latest := make([]speedprobeSummaryProbe, 0, len(samples))
	for _, sample := range samples {
		sources[sample.Source] = struct{}{}
		latest = append(latest, speedprobeSummaryProbe{
			Source:     sample.Source,
			Name:       sample.Name,
			Mbps:       sample.Mbps,
			Status:     speedprobeSampleStatus(sample),
			MeasuredAt: sample.Timestamp,
		})
	}
	sort.SliceStable(latest, func(i, j int) bool {
		if latest[i].Source != latest[j].Source {
			return latest[i].Source < latest[j].Source
		}
		return latest[i].Name < latest[j].Name
	})
	return speedprobeSummaryResponse{
		Sources: len(sources),
		Probes:  len(samples),
		Latest:  latest,
	}
}

func throughputStatus(samples []model.Sample, thresholds config.MonitoringThresholds) throughputStatusResponse {
	sourceMap := make(map[string]*throughputStatusSource)
	issueCount := 0
	overallLevel := "ok"
	for _, sample := range samples {
		sourceName, sourceLabel, sourceType := throughputSourceMetadata(sample)
		if sourceName == "" {
			continue
		}
		level, reason := throughputProbeLevel(sample, thresholds)
		if level != "ok" {
			issueCount++
		}
		if serviceHealthRank(level) > serviceHealthRank(overallLevel) {
			overallLevel = level
		}
		source := sourceMap[sourceName]
		if source == nil {
			source = &throughputStatusSource{
				Name:  sourceName,
				Label: sourceLabel,
				Type:  sourceType,
				Level: "ok",
			}
			if source.Label == "" {
				source.Label = sourceName
			}
			sourceMap[sourceName] = source
		}
		if source.Observer == nil && sample.Observer != nil {
			observer := *sample.Observer
			source.Observer = &observer
		}
		if serviceHealthRank(level) > serviceHealthRank(source.Level) {
			source.Level = level
		}
		label := sample.Label
		if label == "" {
			label = sample.DisplayName
		}
		if label == "" {
			label = labelForName(sample.Name)
		}
		source.Probes = append(source.Probes, throughputStatusProbe{
			Name:            sample.Name,
			Label:           label,
			Level:           level,
			Status:          throughputSampleStatus(sample),
			Reason:          reason,
			Mbps:            sample.Mbps,
			DurationMs:      sample.DurationMs,
			ExpectedBytes:   sample.ExpectedBytes,
			DownloadedBytes: sample.DownloadedBytes,
			ManualOnly:      sample.ManualOnly,
			MeasuredAt:      sample.Timestamp,
			NextCheckAt:     sample.NextCheckAt,
			RetryState:      sample.RetryState,
		})
	}
	sources := make([]throughputStatusSource, 0, len(sourceMap))
	for _, source := range sourceMap {
		sort.SliceStable(source.Probes, func(i, j int) bool {
			return source.Probes[i].Name < source.Probes[j].Name
		})
		sources = append(sources, *source)
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})
	return throughputStatusResponse{
		Level:      overallLevel,
		Alert:      false,
		IssueCount: issueCount,
		Sources:    sources,
	}
}

func throughputStatusSummary(status throughputStatusResponse) throughputStatusSummaryResponse {
	probes := 0
	for _, source := range status.Sources {
		probes += len(source.Probes)
	}
	return throughputStatusSummaryResponse{
		Level:      status.Level,
		Alert:      false,
		IssueCount: status.IssueCount,
		Sources:    len(status.Sources),
		Probes:     probes,
	}
}

func throughputSourceMetadata(sample model.Sample) (string, string, string) {
	switch sample.Type {
	case "download":
		return "legacy_download", "Local Download Probes", "download_probe"
	case "speedprobe":
		label := sample.SourceLabel
		if label == "" {
			label = sample.Source
		}
		return sample.Source, label, "speedprobe"
	default:
		return "", "", ""
	}
}

func throughputProbeLevel(sample model.Sample, thresholds config.MonitoringThresholds) (string, string) {
	switch sample.Type {
	case "download":
		if sample.OK == nil {
			return "unknown", "download_unknown"
		}
		reasons := downloadReasons(sample, thresholds.Download)
		if len(reasons) == 0 {
			return "ok", "none"
		}
		level := levelForReasons(reasons)
		return level, reasons[0].Code
	case "speedprobe":
		level := speedprobeLevel(sample)
		switch level {
		case "ok":
			return level, "none"
		case "unknown":
			return level, "speedprobe_unknown"
		default:
			return level, "speedprobe_failure"
		}
	default:
		return "ok", "none"
	}
}

func throughputSampleStatus(sample model.Sample) string {
	if sample.Type == "speedprobe" {
		return speedprobeSampleStatus(sample)
	}
	if sample.OK == nil {
		return "unknown"
	}
	if *sample.OK {
		return "ok"
	}
	return "failed"
}

func speedprobeLevel(sample model.Sample) string {
	if sample.OK == nil || sample.Status == "unknown" {
		return "unknown"
	}
	if !*sample.OK || sample.Error != "" {
		return "warning"
	}
	return "ok"
}

func speedprobeSampleStatus(sample model.Sample) string {
	if sample.Status != "" {
		return sample.Status
	}
	if sample.OK != nil && *sample.OK {
		return "ok"
	}
	if sample.OK != nil && !*sample.OK {
		return "failed"
	}
	return "unknown"
}
