package api

import (
	"fmt"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

var monitoringHistoryRanges = []string{"1h", "2h", "6h", "24h", "7d"}
var monitoringHistoryBuckets = []string{"5m", "15m", "30m", "1h"}

type monitoringStatusHistorySupportResponse struct {
	Ranges  []string `json:"ranges"`
	Buckets []string `json:"buckets"`
}

type monitoringStatusHistoryResponse struct {
	Source           string                         `json:"source"`
	GeneratedAt      time.Time                      `json:"generated_at"`
	Range            string                         `json:"range"`
	Bucket           string                         `json:"bucket"`
	BucketSeconds    int                            `json:"bucket_seconds"`
	ActualRangeStart time.Time                      `json:"actual_range_start"`
	ActualRangeEnd   time.Time                      `json:"actual_range_end"`
	Points           []monitoringStatusHistoryPoint `json:"points"`
	Summary          monitoringStatusHistorySummary `json:"summary"`
}

type monitoringStatusHistoryPoint struct {
	BucketStart   time.Time `json:"bucket_start"`
	BucketEnd     time.Time `json:"bucket_end"`
	Level         string    `json:"level"`
	Alert         bool      `json:"alert"`
	SampleCount   int       `json:"sample_count"`
	CriticalCount int       `json:"critical_count"`
	WarningCount  int       `json:"warning_count"`
	UnknownCount  int       `json:"unknown_count"`
	OKCount       int       `json:"ok_count"`
}

type monitoringStatusHistorySummary struct {
	OKCount       int `json:"ok_count"`
	WarningCount  int `json:"warning_count"`
	CriticalCount int `json:"critical_count"`
	UnknownCount  int `json:"unknown_count"`
}

func monitoringStatusHistorySupport() monitoringStatusHistorySupportResponse {
	return monitoringStatusHistorySupportResponse{
		Ranges:  append([]string(nil), monitoringHistoryRanges...),
		Buckets: append([]string(nil), monitoringHistoryBuckets...),
	}
}

func parseMonitoringHistoryRange(value string) (time.Duration, error) {
	switch value {
	case "1h":
		return time.Hour, nil
	case "2h":
		return 2 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "24h":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported range: %s", value)
	}
}

func parseMonitoringHistoryBucket(value string) (time.Duration, error) {
	switch value {
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported bucket: %s", value)
	}
}

func buildMonitoringStatusHistory(samples []model.Sample, thresholds config.MonitoringThresholds, rangeValue, bucketValue string, duration, bucket time.Duration, start, end, generatedAt time.Time) monitoringStatusHistoryResponse {
	response := monitoringStatusHistoryResponse{
		Source:           "netwatch",
		GeneratedAt:      generatedAt,
		Range:            rangeValue,
		Bucket:           bucketValue,
		BucketSeconds:    int(bucket.Seconds()),
		ActualRangeStart: start,
		ActualRangeEnd:   end.Add(-time.Second),
	}

	bucketCount := int(duration / bucket)
	response.Points = make([]monitoringStatusHistoryPoint, 0, bucketCount)
	for i := 0; i < bucketCount; i++ {
		bucketStart := start.Add(time.Duration(i) * bucket)
		bucketEnd := bucketStart.Add(bucket)
		point := buildMonitoringStatusHistoryPoint(bucketStart, bucketEnd, samplesInRange(samples, bucketStart, bucketEnd), thresholds)
		response.Points = append(response.Points, point)
		response.Summary.add(point.Level)
	}

	return response
}

func buildMonitoringStatusHistoryPoint(start, end time.Time, samples []model.Sample, thresholds config.MonitoringThresholds) monitoringStatusHistoryPoint {
	point := monitoringStatusHistoryPoint{
		BucketStart: start,
		BucketEnd:   end.Add(-time.Second),
		Level:       "unknown",
	}
	monitoredSamples := filterMonitoringHistorySamples(samples)
	if len(monitoredSamples) == 0 {
		return point
	}

	point.SampleCount = len(monitoredSamples)
	serviceFailures := serviceFailureCounts(monitoredSamples)
	for _, sample := range monitoredSamples {
		switch sampleMonitoringHistoryLevel(sample, thresholds, serviceFailures) {
		case "critical":
			point.CriticalCount++
		case "warning":
			point.WarningCount++
		case "unknown":
			point.UnknownCount++
		default:
			point.OKCount++
		}
	}

	point.Level = levelForHistoryReasons(collectMonitoringReasons(monitoredSamples, thresholds))
	point.Alert = point.Level == "warning" || point.Level == "critical"
	return point
}

func sampleMonitoringHistoryLevel(sample model.Sample, thresholds config.MonitoringThresholds, serviceFailures map[string]int) string {
	var reasons []monitoringReason
	switch sample.Type {
	case "ping":
		reasons = pingReasons(sample, thresholds.Ping)
	case "dns":
		reasons = dnsReasons(sample, thresholds.DNS)
	case "http":
		reasons = httpReasons(sample, thresholds.HTTP, serviceFailures)
	case "download":
		reasons = downloadReasons(sample, thresholds.Download)
	default:
		return "ok"
	}
	return levelForReasons(reasons)
}

func levelForHistoryReasons(reasons []monitoringReason) string {
	if len(reasons) == 0 {
		return "ok"
	}
	return levelForReasons(reasons)
}

func samplesInRange(samples []model.Sample, start, end time.Time) []model.Sample {
	var filtered []model.Sample
	for _, sample := range samples {
		if sample.Timestamp.Before(start) || !sample.Timestamp.Before(end) {
			continue
		}
		filtered = append(filtered, sample)
	}
	return filtered
}

func filterMonitoringHistorySamples(samples []model.Sample) []model.Sample {
	filtered := make([]model.Sample, 0, len(samples))
	for _, sample := range samples {
		if isIgnoredServiceTarget(sample) {
			continue
		}
		filtered = append(filtered, sample)
	}
	return filtered
}

func nextBucketBoundary(value time.Time, bucket time.Duration) time.Time {
	truncated := value.Truncate(bucket)
	if value.Equal(truncated) {
		return truncated
	}
	return truncated.Add(bucket)
}

func (s *monitoringStatusHistorySummary) add(level string) {
	switch level {
	case "critical":
		s.CriticalCount++
	case "warning":
		s.WarningCount++
	case "unknown":
		s.UnknownCount++
	default:
		s.OKCount++
	}
}
