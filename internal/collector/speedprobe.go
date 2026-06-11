package collector

import (
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
	"github.com/youhey/netwatch/internal/speedprobe"
)

func speedprobeSample(source config.RemoteSpeedProbeConfig, observer speedprobe.Observer, probeResult speedprobe.Probe, collectedAt time.Time) (model.Sample, bool) {
	status := strings.ToLower(strings.TrimSpace(probeResult.Status))
	if probeResult.Running || status == "running" || status == "unknown" || probeResult.MeasuredAt == nil {
		return model.Sample{}, false
	}

	ok := status == "ok" && probeResult.Error == nil
	errorValue := ""
	if probeResult.Error != nil {
		errorValue = *probeResult.Error
	}
	sample := model.Sample{
		Timestamp:       *probeResult.MeasuredAt,
		Kind:            "speedprobe",
		Type:            "speedprobe",
		Name:            probeResult.Name,
		Label:           probeResult.Label,
		Source:          source.Name,
		SourceLabel:     source.Label,
		DisplayName:     probeResult.Label,
		DisplayOrder:    source.DisplayOrder,
		OK:              boolPtr(ok),
		Error:           errorValue,
		URL:             probeResult.URL,
		HTTPStatus:      probeResult.HTTPStatusCode,
		DurationMs:      probeResult.DurationMs,
		ExpectedBytes:   probeResult.ExpectedBytes,
		DownloadedBytes: probeResult.DownloadedBytes,
		Mbps:            probeResult.Mbps,
		Observer: &model.SpeedprobeObserver{
			Hostname:  observer.Hostname,
			Interface: observer.Interface,
			LinkSpeed: observer.LinkSpeed,
			Duplex:    observer.Duplex,
			Operstate: observer.Operstate,
		},
		Status:      status,
		Running:     boolPtr(probeResult.Running),
		ManualOnly:  boolPtr(probeResult.ManualOnly),
		Enabled:     boolPtr(probeResult.Enabled),
		RunID:       probeResult.LastRunID,
		CollectedAt: &collectedAt,
	}
	if sample.SourceLabel == "" {
		sample.SourceLabel = source.Name
	}
	if sample.Label == "" {
		sample.Label = probeResult.Name
	}
	if sample.DisplayName == "" {
		sample.DisplayName = sample.Label
	}
	return sample, true
}

func speedprobeDedupeKey(sample model.Sample) string {
	if sample.RunID != "" {
		return sample.Source + "|" + sample.Name + "|" + sample.RunID
	}
	return sample.Source + "|" + sample.Name + "|" + sample.Timestamp.Format(time.RFC3339Nano)
}
