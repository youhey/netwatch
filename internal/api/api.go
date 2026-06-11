package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

const apiVersion = "0.4"

type Handler struct {
	state             *collector.State
	version           string
	targets           []config.TargetConfig
	downloadProbes    []config.DownloadProbeConfig
	remoteSpeedProbes []config.RemoteSpeedProbeConfig
	statusPages       []config.StatusPageConfig
	thresholds        config.MonitoringThresholds
	exportStorage     exportStorageConfig
}

func New(state *collector.State, version string, targets ...[]config.TargetConfig) *Handler {
	var configuredTargets []config.TargetConfig
	if len(targets) > 0 {
		configuredTargets = targets[0]
	}
	return &Handler{
		state:      state,
		version:    version,
		targets:    configuredTargets,
		thresholds: config.DefaultMonitoringThresholds(),
	}
}

func (h *Handler) WithDownloadProbes(downloadProbes []config.DownloadProbeConfig) *Handler {
	h.downloadProbes = downloadProbes
	return h
}

func (h *Handler) WithStatusPages(statusPages []config.StatusPageConfig) *Handler {
	h.statusPages = statusPages
	return h
}

func (h *Handler) WithRemoteSpeedProbes(remoteSpeedProbes []config.RemoteSpeedProbeConfig) *Handler {
	h.remoteSpeedProbes = remoteSpeedProbes
	return h
}

func (h *Handler) WithMonitoringThresholds(thresholds config.MonitoringThresholds) *Handler {
	h.thresholds = thresholds
	return h
}

func (h *Handler) WithExportStorage(dataPath, dataDir, filePattern string) *Handler {
	h.exportStorage = exportStorageConfig{
		DataPath:    dataPath,
		DataDir:     dataDir,
		FilePattern: filePattern,
	}
	return h
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/latest", h.latest)
	mux.HandleFunc("GET /api/ping/latest", h.pingLatest)
	mux.HandleFunc("GET /api/ping/series", h.pingSeries)
	mux.HandleFunc("GET /api/dns/latest", h.dnsLatest)
	mux.HandleFunc("GET /api/dns/series", h.dnsSeries)
	mux.HandleFunc("GET /api/http/latest", h.httpLatest)
	mux.HandleFunc("GET /api/http/series", h.httpSeries)
	mux.HandleFunc("GET /api/download/latest", h.downloadLatest)
	mux.HandleFunc("GET /api/download/series", h.downloadSeries)
	mux.HandleFunc("GET /api/speedprobe/latest", h.speedprobeLatest)
	mux.HandleFunc("GET /api/speedprobe/series", h.speedprobeSeries)
	mux.HandleFunc("GET /api/status-pages/latest", h.statusPagesLatest)
	mux.HandleFunc("GET /api/export/ai", h.aiExport)
	mux.HandleFunc("GET /api/summary", h.summary)
	mux.HandleFunc("GET /api/services/latest", h.servicesLatest)
	mux.HandleFunc("GET /api/services/series", h.servicesSeries)
	mux.HandleFunc("GET /api/services/summary", h.servicesSummary)
	mux.HandleFunc("GET /api/charts/catalog", h.chartsCatalog)
	mux.HandleFunc("GET /api/charts/overview", h.chartsOverview)
	mux.HandleFunc("GET /api/monitoring/status", h.monitoringStatus)
	mux.HandleFunc("GET /api/monitoring/status/history", h.monitoringStatusHistory)
	mux.HandleFunc("GET /api/monitoring/compact", h.monitoringCompact)
	mux.HandleFunc("GET /api/monitoring/thresholds", h.monitoringThresholds)
	mux.HandleFunc("GET /api/capabilities", h.capabilities)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "netwatch",
		"version": h.version,
	})
}

func (h *Handler) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":      "netwatch",
		"version":      h.version,
		"api_version":  apiVersion,
		"generated_at": time.Now(),
		"features": map[string]bool{
			"ping":                            true,
			"dns":                             true,
			"http":                            true,
			"download":                        true,
			"download_series":                 true,
			"speedprobe":                      true,
			"speedprobe_series":               true,
			"summary":                         true,
			"status_pages":                    true,
			"ai_export":                       true,
			"services":                        true,
			"charts":                          true,
			"charts_download":                 true,
			"charts_catalog":                  true,
			"charts_overview":                 true,
			"monitoring_status":               true,
			"monitoring_thresholds":           true,
			"monitoring_status_history":       true,
			"monitoring_status_history_2h_5m": true,
			"monitoring_compact":              true,
		},
		"chart":                     chartSupport(),
		"monitoring_status_history": monitoringStatusHistorySupport(),
		"monitoring_compact":        monitoringCompactSupport(),
	})
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ping":         h.latestByType("ping"),
		"dns":          h.latestByType("dns"),
		"http":         h.latestByType("http"),
		"download":     h.latestByType("download"),
		"speedprobe":   h.latestByType("speedprobe"),
		"status_pages": h.latestByType("status_page"),
	})
}

func (h *Handler) pingLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.latestByType("ping"),
	})
}

func (h *Handler) pingSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "ping")
}

func (h *Handler) dnsLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.latestByType("dns"),
	})
}

func (h *Handler) dnsSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "dns")
}

func (h *Handler) httpLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.latestByType("http"),
	})
}

func (h *Handler) httpSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "http")
}

func (h *Handler) downloadLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.latestByType("download"),
	})
}

func (h *Handler) downloadSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "download")
}

func (h *Handler) speedprobeLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.latestByType("speedprobe"),
	})
}

func (h *Handler) speedprobeSeries(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if source == "" || name == "" {
		writeError(w, http.StatusBadRequest, "source and name are required")
		return
	}
	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseRange(rangeValue)
	if err != nil {
		if r.URL.Query().Get("bucket") != "" {
			writeStructuredError(w, http.StatusBadRequest, "invalid_range", err.Error(), "range", nil)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bucketValue := r.URL.Query().Get("bucket")
	if bucketValue != "" {
		bucket, err := parseBucket(bucketValue)
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_bucket", err.Error(), "bucket", nil)
			return
		}
		maxPoints, err := parseMaxPoints(r.URL.Query().Get("max_points"))
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_max_points", err.Error(), "max_points", maxPointsMeta())
			return
		}
		end := time.Now()
		start := end.Add(-duration)
		samples := h.speedprobeSeriesSamples(source, name, start)
		if len(samples) == 0 {
			writeStructuredError(w, http.StatusNotFound, "target_not_found", "chart series not found", "name", nil)
			return
		}
		writeJSON(w, http.StatusOK, buildChartResponse("speedprobe", rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  source,
		"name":    name,
		"range":   rangeValue,
		"samples": h.speedprobeSeriesSamples(source, name, time.Now().Add(-duration)),
	})
}

func (h *Handler) statusPagesLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusPagesLatestResponse(h.latestStatusPages(), time.Now()))
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	generatedAt := time.Now()
	status := buildMonitoringStatus(h.applyDisplayMetadata(h.state.LatestAll()), h.thresholds, generatedAt)
	throughput := throughputStatus(h.latestThroughputSamples(), h.thresholds)
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":      generatedAt,
		"network_status":    monitoringSummary(status),
		"throughput_status": throughputStatusSummary(throughput),
		"service_health":    serviceHealthSummary(h.latestServices(), h.thresholds),
		"speedprobe":        speedprobeSummary(h.latestSpeedprobes()),
		"provider_status":   providerStatusSummary(h.latestStatusPages()),
	})
}

func (h *Handler) servicesLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"services": groupServiceLatest(h.latestServices(), h.thresholds),
	})
}

func (h *Handler) servicesSeries(w http.ResponseWriter, r *http.Request) {
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if group != "" && name != "" {
		writeError(w, http.StatusBadRequest, "group and name cannot be used together")
		return
	}
	if group == "" && name == "" {
		writeError(w, http.StatusBadRequest, "group or name is required")
		return
	}

	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseRange(rangeValue)
	if err != nil {
		if r.URL.Query().Get("bucket") != "" {
			writeStructuredError(w, http.StatusBadRequest, "invalid_range", err.Error(), "range", nil)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bucketValue := r.URL.Query().Get("bucket")
	if bucketValue != "" {
		bucket, err := parseBucket(bucketValue)
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_bucket", err.Error(), "bucket", nil)
			return
		}
		maxPoints, err := parseMaxPoints(r.URL.Query().Get("max_points"))
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_max_points", err.Error(), "max_points", maxPointsMeta())
			return
		}
		end := time.Now()
		start := end.Add(-duration)
		samples := filterIgnoredServiceTargets(h.serviceSeries(group, name, start))
		if len(samples) == 0 {
			code := "group_not_found"
			param := "group"
			if name != "" {
				code = "target_not_found"
				param = "name"
			}
			writeStructuredError(w, http.StatusNotFound, code, "chart series not found", param, nil)
			return
		}
		writeJSON(w, http.StatusOK, buildServiceChartResponse(group, rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"group":   group,
		"name":    name,
		"range":   rangeValue,
		"samples": filterIgnoredServiceTargets(h.serviceSeries(group, name, time.Now().Add(-duration))),
	})
}

func (h *Handler) servicesSummary(w http.ResponseWriter, r *http.Request) {
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseRange(rangeValue)
	if err != nil {
		if r.URL.Query().Get("bucket") != "" {
			writeStructuredError(w, http.StatusBadRequest, "invalid_range", err.Error(), "range", nil)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"range":  rangeValue,
		"groups": summarizeServices(h.serviceSeries(group, "", time.Now().Add(-duration))),
	})
}

func (h *Handler) series(w http.ResponseWriter, r *http.Request, sampleType string) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseRange(rangeValue)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bucketValue := r.URL.Query().Get("bucket")
	if bucketValue != "" {
		bucket, err := parseBucket(bucketValue)
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_bucket", err.Error(), "bucket", nil)
			return
		}
		maxPoints, err := parseMaxPoints(r.URL.Query().Get("max_points"))
		if err != nil {
			writeStructuredError(w, http.StatusBadRequest, "invalid_max_points", err.Error(), "max_points", maxPointsMeta())
			return
		}
		end := time.Now()
		start := end.Add(-duration)
		samples := h.seriesByType(sampleType, name, start)
		if len(samples) == 0 {
			writeStructuredError(w, http.StatusNotFound, "target_not_found", "chart series not found", "name", nil)
			return
		}
		writeJSON(w, http.StatusOK, buildChartResponse(sampleType, rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":                        name,
		"range":                       rangeValue,
		seriesResponseKey(sampleType): h.seriesByType(sampleType, name, time.Now().Add(-duration)),
	})
}

func (h *Handler) monitoringStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildMonitoringStatus(h.applyDisplayMetadata(h.state.LatestAll()), h.thresholds, time.Now()))
}

func (h *Handler) monitoringThresholds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, monitoringThresholdsResponse(h.thresholds, time.Now()))
}

func (h *Handler) monitoringStatusHistory(w http.ResponseWriter, r *http.Request) {
	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseMonitoringHistoryRange(rangeValue)
	if err != nil {
		writeStructuredError(w, http.StatusBadRequest, "invalid_range", err.Error(), "range", nil)
		return
	}

	bucketValue := r.URL.Query().Get("bucket")
	if bucketValue == "" {
		bucketValue = "1h"
	}
	bucket, err := parseMonitoringHistoryBucket(bucketValue)
	if err != nil {
		writeStructuredError(w, http.StatusBadRequest, "invalid_bucket", err.Error(), "bucket", nil)
		return
	}

	generatedAt := time.Now()
	end := nextBucketBoundary(generatedAt, bucket)
	start := end.Add(-duration)
	samples := h.applyDisplayMetadata(h.state.SamplesSince(start))
	writeJSON(w, http.StatusOK, buildMonitoringStatusHistory(samples, h.thresholds, rangeValue, bucketValue, duration, bucket, start, end, generatedAt))
}

func (h *Handler) monitoringCompact(w http.ResponseWriter, r *http.Request) {
	generatedAt := time.Now()
	duration := 2 * time.Hour
	bucket := 5 * time.Minute
	end := nextBucketBoundary(generatedAt, bucket)
	start := end.Add(-duration)
	status := buildMonitoringStatus(h.applyDisplayMetadata(h.state.LatestAll()), h.thresholds, generatedAt)
	history := buildMonitoringStatusHistory(h.applyDisplayMetadata(h.state.SamplesSince(start)), h.thresholds, "2h", "5m", duration, bucket, start, end, generatedAt)
	writeJSON(w, http.StatusOK, buildMonitoringCompact(status, history, generatedAt, h.thresholds, h.latestServices(), h.latestThroughputSamples(), h.latestStatusPages()))
}

func (h *Handler) latestByType(sampleType string) []model.Sample {
	return h.applyDisplayMetadata(h.state.LatestByType(sampleType))
}

func (h *Handler) seriesByType(sampleType, name string, since time.Time) []model.Sample {
	return h.applyDisplayMetadata(h.state.SeriesByType(sampleType, name, since))
}

func (h *Handler) latestServices() []model.Sample {
	return h.applyDisplayMetadata(h.state.LatestServices())
}

func (h *Handler) latestStatusPages() []model.Sample {
	return h.applyDisplayMetadata(h.state.LatestByType("status_page"))
}

func (h *Handler) latestSpeedprobes() []model.Sample {
	return h.applyDisplayMetadata(h.state.LatestByType("speedprobe"))
}

func (h *Handler) latestThroughputSamples() []model.Sample {
	samples := h.latestByType("download")
	samples = append(samples, h.latestSpeedprobes()...)
	return samples
}

func (h *Handler) speedprobeSeriesSamples(source, name string, since time.Time) []model.Sample {
	return h.applyDisplayMetadata(h.state.SeriesByTypeSource("speedprobe", source, name, since))
}

func (h *Handler) serviceSeries(group, name string, since time.Time) []model.Sample {
	return h.applyDisplayMetadata(h.state.ServiceSeries(group, name, since))
}

func (h *Handler) applyDisplayMetadata(samples []model.Sample) []model.Sample {
	ordered := append([]model.Sample(nil), samples...)
	for i := range ordered {
		if ordered[i].DisplayOrder > 0 {
			ordered[i].DisplayName = h.displayNameFor(ordered[i])
		} else {
			ordered[i].DisplayOrder = h.displayOrderForSample(ordered[i])
			ordered[i].DisplayName = h.displayNameFor(ordered[i])
		}
	}
	sortSamplesForDisplay(ordered)
	return ordered
}

func (h *Handler) displayOrderForSample(sample model.Sample) int {
	if sample.Type == "speedprobe" {
		for _, remoteSpeedProbe := range h.remoteSpeedProbes {
			if remoteSpeedProbe.Name == sample.Source {
				return remoteSpeedProbe.DisplayOrder
			}
		}
	}
	return h.displayOrderFor(sample.Name)
}

func (h *Handler) displayOrderFor(name string) int {
	for _, target := range h.targets {
		if target.Name == name {
			return target.DisplayOrder
		}
	}
	for _, probe := range h.downloadProbes {
		if probe.Name == name {
			return probe.DisplayOrder
		}
	}
	for _, remoteSpeedProbe := range h.remoteSpeedProbes {
		if remoteSpeedProbe.Name == name {
			return remoteSpeedProbe.DisplayOrder
		}
	}
	for _, statusPage := range h.statusPages {
		if statusPage.Name == name {
			return statusPage.DisplayOrder
		}
	}
	return 0
}

func (h *Handler) displayNameFor(sample model.Sample) string {
	for _, target := range h.targets {
		if target.Name == sample.Name {
			if strings.TrimSpace(target.Label) != "" {
				return target.Label
			}
			return labelForName(target.Name)
		}
	}
	for _, probe := range h.downloadProbes {
		if probe.Name == sample.Name {
			if strings.TrimSpace(probe.Label) != "" {
				return probe.Label
			}
			return labelForName(probe.Name)
		}
	}
	for _, remoteSpeedProbe := range h.remoteSpeedProbes {
		if remoteSpeedProbe.Name == sample.Source {
			if strings.TrimSpace(sample.DisplayName) != "" {
				return sample.DisplayName
			}
			if strings.TrimSpace(sample.Label) != "" {
				return sample.Label
			}
			return labelForName(sample.Name)
		}
	}
	for _, statusPage := range h.statusPages {
		if statusPage.Name == sample.Name {
			if strings.TrimSpace(statusPage.Label) != "" {
				return statusPage.Label
			}
			return labelForName(statusPage.Name)
		}
	}
	if strings.TrimSpace(sample.DisplayName) != "" {
		return sample.DisplayName
	}
	return labelForName(sample.Name)
}

func parseRange(value string) (time.Duration, error) {
	switch value {
	case "1h":
		return time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "24h":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	case "14d":
		return 14 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported range: %s", value)
	}
}

type monitoringStatusResponse struct {
	Alert         bool               `json:"alert"`
	Source        string             `json:"source"`
	StatusID      string             `json:"status_id"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Level         string             `json:"level"`
	Title         string             `json:"title"`
	Message       string             `json:"message"`
	PrimaryReason *monitoringReason  `json:"primary_reason"`
	Reasons       []monitoringReason `json:"reasons"`
}

type monitoringSummaryResponse struct {
	Level      string `json:"level"`
	Alert      bool   `json:"alert"`
	IssueCount int    `json:"issue_count"`
}

func monitoringSummary(status monitoringStatusResponse) monitoringSummaryResponse {
	return monitoringSummaryResponse{
		Level:      status.Level,
		Alert:      status.Alert,
		IssueCount: len(status.Reasons),
	}
}

type serviceGroupResponse struct {
	Group       string         `json:"group"`
	DisplayName string         `json:"display_name"`
	Category    string         `json:"category,omitempty"`
	Status      string         `json:"status"`
	Targets     []model.Sample `json:"targets"`
}

type serviceSummaryResponse struct {
	Group        string  `json:"group"`
	DisplayName  string  `json:"display_name"`
	Category     string  `json:"category,omitempty"`
	SampleCount  int     `json:"sample_count"`
	OKCount      int     `json:"ok_count"`
	OKRate       float64 `json:"ok_rate"`
	AvgTotalMs   float64 `json:"avg_total_ms"`
	MaxTotalMs   float64 `json:"max_total_ms"`
	AvgDNSMs     float64 `json:"avg_dns_ms,omitempty"`
	AvgConnectMs float64 `json:"avg_connect_ms,omitempty"`
	AvgTLSMs     float64 `json:"avg_tls_ms,omitempty"`
	AvgTTFBMs    float64 `json:"avg_ttfb_ms,omitempty"`
	MaxTTFBMs    float64 `json:"max_ttfb_ms,omitempty"`
	TimeoutCount int     `json:"timeout_count"`
	ErrorCount   int     `json:"error_count"`
}

func groupServiceLatest(samples []model.Sample, thresholds config.MonitoringThresholds) []serviceGroupResponse {
	groups := make(map[string]*serviceGroupResponse)
	serviceFailures := serviceFailureCounts(samples)
	for _, sample := range samples {
		if isIgnoredServiceTarget(sample) {
			continue
		}
		group := sample.Group
		if group == "" {
			continue
		}
		if _, ok := groups[group]; !ok {
			groups[group] = &serviceGroupResponse{
				Group:       group,
				DisplayName: labelForName(group),
				Category:    sample.Category,
				Status:      "ok",
			}
		}
		entry := groups[group]
		if entry.Category == "" {
			entry.Category = sample.Category
		}
		entry.Targets = append(entry.Targets, sample)
		level := levelForReasons(httpReasons(sample, thresholds.HTTP, serviceFailures))
		if severityRank(level) > severityRank(entry.Status) {
			entry.Status = level
		}
	}

	result := make([]serviceGroupResponse, 0, len(groups))
	for _, group := range groups {
		sortSamplesForDisplay(group.Targets)
		result = append(result, *group)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Group < result[j].Group
	})

	return result
}

func summarizeServices(samples []model.Sample) []serviceSummaryResponse {
	type aggregate struct {
		category     string
		sampleCount  int
		okCount      int
		totalCount   int
		totalSum     float64
		maxTotal     float64
		dnsCount     int
		dnsSum       float64
		connectCount int
		connectSum   float64
		tlsCount     int
		tlsSum       float64
		ttfbCount    int
		ttfbSum      float64
		maxTTFB      float64
		timeoutCount int
		errorCount   int
	}

	aggregates := make(map[string]*aggregate)
	for _, sample := range samples {
		if sample.Group == "" || isIgnoredServiceTarget(sample) {
			continue
		}
		if _, ok := aggregates[sample.Group]; !ok {
			aggregates[sample.Group] = &aggregate{category: sample.Category}
		}
		agg := aggregates[sample.Group]
		if agg.category == "" {
			agg.category = sample.Category
		}
		agg.sampleCount++
		if sample.OK != nil && *sample.OK {
			agg.okCount++
		}
		if sample.TotalMs != nil && *sample.TotalMs > 0 {
			agg.totalCount++
			agg.totalSum += *sample.TotalMs
			if *sample.TotalMs > agg.maxTotal {
				agg.maxTotal = *sample.TotalMs
			}
		}
		if sample.DNSMs != nil {
			agg.dnsCount++
			agg.dnsSum += *sample.DNSMs
		}
		if sample.ConnectMs != nil {
			agg.connectCount++
			agg.connectSum += *sample.ConnectMs
		}
		if sample.TLSMs != nil {
			agg.tlsCount++
			agg.tlsSum += *sample.TLSMs
		}
		if sample.TTFBMs != nil {
			agg.ttfbCount++
			agg.ttfbSum += *sample.TTFBMs
			if *sample.TTFBMs > agg.maxTTFB {
				agg.maxTTFB = *sample.TTFBMs
			}
		}
		if sample.Error != "" || sample.OK != nil && !*sample.OK {
			agg.errorCount++
			if isTimeoutError(sample.Error) {
				agg.timeoutCount++
			}
		}
	}

	result := make([]serviceSummaryResponse, 0, len(aggregates))
	for group, agg := range aggregates {
		okRate := 0.0
		if agg.sampleCount > 0 {
			okRate = float64(agg.okCount) / float64(agg.sampleCount) * 100
		}
		avgTotal := 0.0
		if agg.totalCount > 0 {
			avgTotal = agg.totalSum / float64(agg.totalCount)
		}
		result = append(result, serviceSummaryResponse{
			Group:        group,
			DisplayName:  labelForName(group),
			Category:     agg.category,
			SampleCount:  agg.sampleCount,
			OKCount:      agg.okCount,
			OKRate:       okRate,
			AvgTotalMs:   avgTotal,
			MaxTotalMs:   agg.maxTotal,
			AvgDNSMs:     avgMetric(agg.dnsSum, agg.dnsCount),
			AvgConnectMs: avgMetric(agg.connectSum, agg.connectCount),
			AvgTLSMs:     avgMetric(agg.tlsSum, agg.tlsCount),
			AvgTTFBMs:    avgMetric(agg.ttfbSum, agg.ttfbCount),
			MaxTTFBMs:    agg.maxTTFB,
			TimeoutCount: agg.timeoutCount,
			ErrorCount:   agg.errorCount,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Group < result[j].Group
	})

	return result
}

func sortSamplesForDisplay(samples []model.Sample) {
	sort.SliceStable(samples, func(i, j int) bool {
		leftOrder := displayOrderRank(samples[i].DisplayOrder)
		rightOrder := displayOrderRank(samples[j].DisplayOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return samples[i].Name < samples[j].Name
	})
}

func isTimeoutError(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "timeout") || strings.Contains(value, "deadline exceeded")
}

func isIgnoredServiceTarget(sample model.Sample) bool {
	return isIgnoredTargetName(sample.Name)
}

func isIgnoredTargetName(name string) bool {
	return name == "sf6_buckler_info"
}

func filterIgnoredServiceTargets(samples []model.Sample) []model.Sample {
	var filtered []model.Sample
	for _, sample := range samples {
		if isIgnoredServiceTarget(sample) {
			continue
		}
		filtered = append(filtered, sample)
	}
	return filtered
}

func avgMetric(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func severityRank(level string) int {
	switch level {
	case "critical":
		return 3
	case "warning":
		return 2
	case "ok":
		return 1
	default:
		return 0
	}
}

func seriesResponseKey(sampleType string) string {
	if sampleType == "download" {
		return "points"
	}
	return "samples"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func writeStructuredError(w http.ResponseWriter, status int, code, message, param string, extra map[string]any) {
	body := map[string]any{
		"code":    code,
		"message": message,
	}
	if param != "" {
		body["param"] = param
	}
	for key, value := range extra {
		body[key] = value
	}
	writeJSON(w, status, map[string]any{
		"error": body,
	})
}

func maxPointsMeta() map[string]any {
	return map[string]any{
		"min": minMaxPoints,
		"max": maxMaxPoints,
	}
}
