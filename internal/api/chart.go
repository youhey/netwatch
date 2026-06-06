package api

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

const (
	defaultMaxPoints = 500
	minMaxPoints     = 10
	maxMaxPoints     = 2000
)

type chartPoint struct {
	Timestamp   time.Time `json:"ts"`
	SampleCount int       `json:"sample_count"`

	AvgMs       *float64 `json:"avg_ms,omitempty"`
	MinMs       *float64 `json:"min_ms,omitempty"`
	MaxMs       *float64 `json:"max_ms,omitempty"`
	LossPercent *float64 `json:"loss_percent,omitempty"`

	AvgTotalMs *float64 `json:"avg_total_ms,omitempty"`
	AvgTTFBMs  *float64 `json:"avg_ttfb_ms,omitempty"`
	MaxTotalMs *float64 `json:"max_total_ms,omitempty"`
	OKRate     *float64 `json:"ok_rate,omitempty"`

	AvgMbps *float64 `json:"avg_mbps,omitempty"`
	MinMbps *float64 `json:"min_mbps,omitempty"`
	MaxMbps *float64 `json:"max_mbps,omitempty"`

	FailureCount int `json:"failure_count"`
	TimeoutCount int `json:"timeout_count"`
}

type chartResponse struct {
	GeneratedAt      time.Time `json:"generated_at"`
	ActualRangeStart time.Time `json:"actual_range_start"`
	ActualRangeEnd   time.Time `json:"actual_range_end"`
	Timezone         string    `json:"timezone"`
	Range            string    `json:"range"`
	Bucket           string    `json:"bucket"`
	BucketSeconds    int       `json:"bucket_seconds"`
	MaxPoints        int       `json:"max_points"`

	Type         string       `json:"type"`
	Name         string       `json:"name,omitempty"`
	DisplayName  string       `json:"display_name,omitempty"`
	Target       string       `json:"target,omitempty"`
	Hostname     string       `json:"hostname,omitempty"`
	Group        string       `json:"group,omitempty"`
	Category     string       `json:"category,omitempty"`
	DisplayOrder int          `json:"display_order,omitempty"`
	URL          string       `json:"url,omitempty"`
	Targets      []string     `json:"targets,omitempty"`
	Points       []chartPoint `json:"points"`
}

func parseBucket(value string) (time.Duration, error) {
	switch value {
	case "1m":
		return time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported bucket: %s", value)
	}
}

func parseMaxPoints(value string) (int, error) {
	if value == "" {
		return defaultMaxPoints, nil
	}
	maxPoints, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid max_points: %s", value)
	}
	if maxPoints < minMaxPoints || maxPoints > maxMaxPoints {
		return 0, fmt.Errorf("max_points must be between %d and %d", minMaxPoints, maxMaxPoints)
	}
	return maxPoints, nil
}

func buildChartResponse(sampleType, rangeValue, bucketValue string, bucket time.Duration, maxPoints int, start, end time.Time, samples []model.Sample) chartResponse {
	response := chartResponse{
		GeneratedAt:      time.Now(),
		ActualRangeStart: start,
		ActualRangeEnd:   end,
		Timezone:         time.Now().Location().String(),
		Range:            rangeValue,
		Bucket:           bucketValue,
		BucketSeconds:    int(bucket.Seconds()),
		MaxPoints:        maxPoints,
		Type:             sampleType,
	}
	if len(samples) == 0 {
		return response
	}

	switch sampleType {
	case "ping":
		response.Name = samples[0].Name
		response.DisplayName = samples[0].DisplayName
		response.DisplayOrder = samples[0].DisplayOrder
		response.Target = samples[0].Target
		response.Points = aggregatePing(samples, bucket)
	case "dns":
		response.Name = samples[0].Name
		response.DisplayName = samples[0].DisplayName
		response.DisplayOrder = samples[0].DisplayOrder
		response.Hostname = samples[0].Hostname
		response.Points = aggregateDNS(samples, bucket)
	case "http":
		response.Name = samples[0].Name
		response.DisplayName = samples[0].DisplayName
		response.Group = samples[0].Group
		response.Category = samples[0].Category
		response.DisplayOrder = samples[0].DisplayOrder
		response.URL = samples[0].URL
		response.Points = aggregateHTTP(samples, bucket)
	case "download":
		response.Name = samples[0].Name
		response.DisplayName = samples[0].DisplayName
		response.DisplayOrder = samples[0].DisplayOrder
		response.URL = samples[0].URL
		response.Points = aggregateDownload(samples, bucket)
	}
	response.Points = thinPoints(response.Points, maxPoints)
	return response
}

func buildServiceChartResponse(group, rangeValue, bucketValue string, bucket time.Duration, maxPoints int, start, end time.Time, samples []model.Sample) chartResponse {
	response := chartResponse{
		GeneratedAt:      time.Now(),
		ActualRangeStart: start,
		ActualRangeEnd:   end,
		Timezone:         time.Now().Location().String(),
		Range:            rangeValue,
		Bucket:           bucketValue,
		BucketSeconds:    int(bucket.Seconds()),
		MaxPoints:        maxPoints,
		Type:             "service_group",
		Group:            group,
	}
	if len(samples) == 0 {
		return response
	}
	if response.Group == "" {
		response.Group = samples[0].Group
	}
	response.DisplayName = labelForName(response.Group)
	response.Category = samples[0].Category
	response.DisplayOrder = minSampleDisplayOrder(samples)
	response.Targets = serviceTargets(samples)
	response.Points = thinPoints(aggregateServices(samples, bucket), maxPoints)
	return response
}

type bucketAggregate struct {
	ts time.Time

	sampleCount int
	okCount     int
	failures    int
	timeouts    int

	avgSum   float64
	avgCount int
	minValue *float64
	maxValue *float64

	totalSum   float64
	totalCount int
	maxTotal   *float64
	ttfbSum    float64
	ttfbCount  int

	sent      int
	received  int
	lossSum   float64
	lossCount int
}

func aggregatePing(samples []model.Sample, bucket time.Duration) []chartPoint {
	buckets := newBuckets(samples, bucket)
	for _, sample := range samples {
		agg := buckets[bucketStart(sample.Timestamp, bucket)]
		agg.sampleCount++
		if sample.RTTAvgMs != nil {
			agg.avgSum += *sample.RTTAvgMs
			agg.avgCount++
		}
		if sample.RTTMinMs != nil {
			agg.minValue = minPtr(agg.minValue, *sample.RTTMinMs)
		}
		if sample.RTTMaxMs != nil {
			agg.maxValue = maxPtr(agg.maxValue, *sample.RTTMaxMs)
		}
		agg.sent += sample.Sent
		agg.received += sample.Received
		if sample.LossPercent != nil {
			agg.lossSum += *sample.LossPercent
			agg.lossCount++
		}
	}

	points := make([]chartPoint, 0, len(buckets))
	for _, agg := range sortedBuckets(buckets) {
		point := chartPoint{
			Timestamp:   agg.ts,
			SampleCount: agg.sampleCount,
			AvgMs:       avgPtr(agg.avgSum, agg.avgCount),
			MinMs:       agg.minValue,
			MaxMs:       agg.maxValue,
		}
		if agg.sent > 0 {
			loss := float64(agg.sent-agg.received) / float64(agg.sent) * 100
			point.LossPercent = &loss
		} else {
			point.LossPercent = avgPtr(agg.lossSum, agg.lossCount)
		}
		points = append(points, point)
	}
	return points
}

func aggregateDNS(samples []model.Sample, bucket time.Duration) []chartPoint {
	buckets := newBuckets(samples, bucket)
	for _, sample := range samples {
		agg := buckets[bucketStart(sample.Timestamp, bucket)]
		agg.sampleCount++
		if sample.OK != nil && !*sample.OK || sample.Error != "" {
			agg.failures++
			continue
		}
		if sample.DurationMs != nil {
			agg.avgSum += *sample.DurationMs
			agg.avgCount++
			agg.minValue = minPtr(agg.minValue, *sample.DurationMs)
			agg.maxValue = maxPtr(agg.maxValue, *sample.DurationMs)
		}
	}

	points := make([]chartPoint, 0, len(buckets))
	for _, agg := range sortedBuckets(buckets) {
		points = append(points, chartPoint{
			Timestamp:    agg.ts,
			SampleCount:  agg.sampleCount,
			AvgMs:        avgPtr(agg.avgSum, agg.avgCount),
			MinMs:        agg.minValue,
			MaxMs:        agg.maxValue,
			FailureCount: agg.failures,
		})
	}
	return points
}

func aggregateHTTP(samples []model.Sample, bucket time.Duration) []chartPoint {
	buckets := newBuckets(samples, bucket)
	for _, sample := range samples {
		agg := buckets[bucketStart(sample.Timestamp, bucket)]
		addHTTPToAggregate(agg, sample)
	}
	return httpPointsFromBuckets(buckets, false)
}

func aggregateServices(samples []model.Sample, bucket time.Duration) []chartPoint {
	buckets := newBuckets(samples, bucket)
	for _, sample := range samples {
		agg := buckets[bucketStart(sample.Timestamp, bucket)]
		addHTTPToAggregate(agg, sample)
	}
	return httpPointsFromBuckets(buckets, true)
}

func aggregateDownload(samples []model.Sample, bucket time.Duration) []chartPoint {
	buckets := newBuckets(samples, bucket)
	for _, sample := range samples {
		agg := buckets[bucketStart(sample.Timestamp, bucket)]
		agg.sampleCount++
		if sample.OK != nil && !*sample.OK || sample.Error != "" {
			agg.failures++
			if isTimeoutError(sample.Error) {
				agg.timeouts++
			}
			continue
		}
		if sample.Mbps != nil {
			agg.avgSum += *sample.Mbps
			agg.avgCount++
			agg.minValue = minPtr(agg.minValue, *sample.Mbps)
			agg.maxValue = maxPtr(agg.maxValue, *sample.Mbps)
		}
	}

	points := make([]chartPoint, 0, len(buckets))
	for _, agg := range sortedBuckets(buckets) {
		points = append(points, chartPoint{
			Timestamp:    agg.ts,
			SampleCount:  agg.sampleCount,
			AvgMbps:      avgPtr(agg.avgSum, agg.avgCount),
			MinMbps:      agg.minValue,
			MaxMbps:      agg.maxValue,
			FailureCount: agg.failures,
			TimeoutCount: agg.timeouts,
		})
	}
	return points
}

func addHTTPToAggregate(agg *bucketAggregate, sample model.Sample) {
	agg.sampleCount++
	if sample.OK != nil && *sample.OK {
		agg.okCount++
	}
	if sample.OK != nil && !*sample.OK || sample.Error != "" {
		agg.failures++
		if isTimeoutError(sample.Error) {
			agg.timeouts++
		}
		return
	}
	if sample.TotalMs != nil && *sample.TotalMs > 0 {
		agg.totalSum += *sample.TotalMs
		agg.totalCount++
		agg.maxTotal = maxPtr(agg.maxTotal, *sample.TotalMs)
	}
	if sample.TTFBMs != nil {
		agg.ttfbSum += *sample.TTFBMs
		agg.ttfbCount++
	}
}

func httpPointsFromBuckets(buckets map[time.Time]*bucketAggregate, includeOKRate bool) []chartPoint {
	points := make([]chartPoint, 0, len(buckets))
	for _, agg := range sortedBuckets(buckets) {
		point := chartPoint{
			Timestamp:    agg.ts,
			SampleCount:  agg.sampleCount,
			AvgTotalMs:   avgPtr(agg.totalSum, agg.totalCount),
			AvgTTFBMs:    avgPtr(agg.ttfbSum, agg.ttfbCount),
			MaxTotalMs:   agg.maxTotal,
			FailureCount: agg.failures,
			TimeoutCount: agg.timeouts,
		}
		if includeOKRate {
			okRate := 0.0
			if agg.sampleCount > 0 {
				okRate = float64(agg.okCount) / float64(agg.sampleCount) * 100
			}
			point.OKRate = &okRate
		}
		points = append(points, point)
	}
	return points
}

func newBuckets(samples []model.Sample, bucket time.Duration) map[time.Time]*bucketAggregate {
	buckets := make(map[time.Time]*bucketAggregate)
	for _, sample := range samples {
		ts := bucketStart(sample.Timestamp, bucket)
		if _, ok := buckets[ts]; !ok {
			buckets[ts] = &bucketAggregate{ts: ts}
		}
	}
	return buckets
}

func sortedBuckets(buckets map[time.Time]*bucketAggregate) []*bucketAggregate {
	values := make([]*bucketAggregate, 0, len(buckets))
	for _, agg := range buckets {
		values = append(values, agg)
	}
	sort.SliceStable(values, func(i, j int) bool {
		return values[i].ts.Before(values[j].ts)
	})
	return values
}

func bucketStart(ts time.Time, bucket time.Duration) time.Time {
	return ts.Truncate(bucket)
}

func avgPtr(sum float64, count int) *float64 {
	if count == 0 {
		return nil
	}
	avg := sum / float64(count)
	return &avg
}

func minPtr(current *float64, value float64) *float64 {
	if current == nil || value < *current {
		return &value
	}
	return current
}

func maxPtr(current *float64, value float64) *float64 {
	if current == nil || value > *current {
		return &value
	}
	return current
}

func thinPoints(points []chartPoint, maxPoints int) []chartPoint {
	if len(points) <= maxPoints {
		return points
	}
	thinned := make([]chartPoint, 0, maxPoints)
	step := float64(len(points)) / float64(maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(math.Floor(float64(i) * step))
		if idx >= len(points) {
			idx = len(points) - 1
		}
		thinned = append(thinned, points[idx])
	}
	return thinned
}

func serviceTargets(samples []model.Sample) []string {
	seen := make(map[string]struct{})
	orders := make(map[string]int)
	for _, sample := range samples {
		seen[sample.Name] = struct{}{}
		if shouldReplaceDisplayOrder(orders[sample.Name], sample.DisplayOrder) {
			orders[sample.Name] = sample.DisplayOrder
		}
	}
	targets := make([]string, 0, len(seen))
	for target := range seen {
		targets = append(targets, target)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		leftOrder := displayOrderRank(orders[targets[i]])
		rightOrder := displayOrderRank(orders[targets[j]])
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return targets[i] < targets[j]
	})
	return targets
}

func minSampleDisplayOrder(samples []model.Sample) int {
	minOrder := 0
	for _, sample := range samples {
		if sample.DisplayOrder <= 0 {
			continue
		}
		if minOrder == 0 || sample.DisplayOrder < minOrder {
			minOrder = sample.DisplayOrder
		}
	}
	return minOrder
}
