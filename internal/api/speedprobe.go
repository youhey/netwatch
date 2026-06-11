package api

import (
	"sort"
	"time"

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
	Name     string                  `json:"name"`
	Label    string                  `json:"label"`
	Observer string                  `json:"observer,omitempty"`
	Level    string                  `json:"level"`
	Probes   []throughputStatusProbe `json:"probes"`
}

type throughputStatusProbe struct {
	Name       string    `json:"name"`
	Label      string    `json:"label"`
	Status     string    `json:"status"`
	Mbps       *float64  `json:"mbps,omitempty"`
	MeasuredAt time.Time `json:"measured_at"`
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

func throughputStatus(samples []model.Sample) throughputStatusResponse {
	sourceMap := make(map[string]*throughputStatusSource)
	issueCount := 0
	overallLevel := "ok"
	for _, sample := range samples {
		level := speedprobeLevel(sample)
		if level != "ok" {
			issueCount++
		}
		if serviceHealthRank(level) > serviceHealthRank(overallLevel) {
			overallLevel = level
		}
		source := sourceMap[sample.Source]
		if source == nil {
			source = &throughputStatusSource{
				Name:  sample.Source,
				Label: sample.SourceLabel,
				Level: "ok",
			}
			if source.Label == "" {
				source.Label = sample.Source
			}
			if sample.Observer != nil {
				source.Observer = sample.Observer.Hostname
			}
			sourceMap[sample.Source] = source
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
			Name:       sample.Name,
			Label:      label,
			Status:     speedprobeSampleStatus(sample),
			Mbps:       sample.Mbps,
			MeasuredAt: sample.Timestamp,
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
