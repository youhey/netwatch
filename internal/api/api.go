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
	state   *collector.State
	version string
	targets []config.TargetConfig
}

func New(state *collector.State, version string, targets ...[]config.TargetConfig) *Handler {
	var configuredTargets []config.TargetConfig
	if len(targets) > 0 {
		configuredTargets = targets[0]
	}
	return &Handler{
		state:   state,
		version: version,
		targets: configuredTargets,
	}
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
	mux.HandleFunc("GET /api/services/latest", h.servicesLatest)
	mux.HandleFunc("GET /api/services/series", h.servicesSeries)
	mux.HandleFunc("GET /api/services/summary", h.servicesSummary)
	mux.HandleFunc("GET /api/charts/catalog", h.chartsCatalog)
	mux.HandleFunc("GET /api/charts/overview", h.chartsOverview)
	mux.HandleFunc("GET /api/monitoring/status", h.monitoringStatus)
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
			"ping":                  true,
			"dns":                   true,
			"http":                  true,
			"services":              true,
			"charts":                true,
			"charts_catalog":        true,
			"charts_overview":       true,
			"monitoring_status":     true,
			"monitoring_thresholds": true,
		},
		"chart": chartSupport(),
	})
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ping": h.state.LatestByType("ping"),
		"dns":  h.state.LatestByType("dns"),
		"http": h.state.LatestByType("http"),
	})
}

func (h *Handler) pingLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.state.LatestByType("ping"),
	})
}

func (h *Handler) pingSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "ping")
}

func (h *Handler) dnsLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.state.LatestByType("dns"),
	})
}

func (h *Handler) dnsSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "dns")
}

func (h *Handler) httpLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": h.state.LatestByType("http"),
	})
}

func (h *Handler) httpSeries(w http.ResponseWriter, r *http.Request) {
	h.series(w, r, "http")
}

func (h *Handler) servicesLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"services": groupServiceLatest(h.state.LatestServices()),
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
		samples := filterIgnoredServiceTargets(h.state.ServiceSeries(group, name, start))
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
		"samples": filterIgnoredServiceTargets(h.state.ServiceSeries(group, name, time.Now().Add(-duration))),
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
		"groups": summarizeServices(h.state.ServiceSeries(group, "", time.Now().Add(-duration))),
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
		samples := h.state.SeriesByType(sampleType, name, start)
		if len(samples) == 0 {
			writeStructuredError(w, http.StatusNotFound, "target_not_found", "chart series not found", "name", nil)
			return
		}
		writeJSON(w, http.StatusOK, buildChartResponse(sampleType, rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":    name,
		"range":   rangeValue,
		"samples": h.state.SeriesByType(sampleType, name, time.Now().Add(-duration)),
	})
}

func (h *Handler) monitoringStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildMonitoringStatus(h.state.LatestAll()))
}

func (h *Handler) monitoringThresholds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now(),
		"ping": map[string]any{
			"external_rtt_avg_ms":   map[string]float64{"warning": 100, "critical": 200},
			"external_loss_percent": map[string]float64{"warning": 1, "critical": 5},
			"gateway_loss_percent":  map[string]float64{"warning": 0, "critical": 0},
		},
		"dns": map[string]any{
			"duration_ms": map[string]float64{"warning": 300, "critical": 1000},
		},
		"http": map[string]any{
			"total_ms": map[string]float64{"warning": 3000, "critical": 5000},
		},
		"service": map[string]any{
			"ok_rate_percent": map[string]float64{"warning": 95, "critical": 90},
		},
	})
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
	Alert   bool   `json:"alert"`
	Source  string `json:"source"`
	Level   string `json:"level"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

type serviceGroupResponse struct {
	Group    string         `json:"group"`
	Category string         `json:"category,omitempty"`
	Status   string         `json:"status"`
	Targets  []model.Sample `json:"targets"`
}

type serviceSummaryResponse struct {
	Group        string  `json:"group"`
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

func groupServiceLatest(samples []model.Sample) []serviceGroupResponse {
	groups := make(map[string]*serviceGroupResponse)
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
				Group:    group,
				Category: sample.Category,
				Status:   "ok",
			}
		}
		entry := groups[group]
		if entry.Category == "" {
			entry.Category = sample.Category
		}
		entry.Targets = append(entry.Targets, sample)
		level, _ := sampleStatus(sample)
		if severityRank(level) > severityRank(entry.Status) {
			entry.Status = level
		}
	}

	result := make([]serviceGroupResponse, 0, len(groups))
	for _, group := range groups {
		sortSamplesByName(group.Targets)
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

func buildMonitoringStatus(samples []model.Sample) monitoringStatusResponse {
	if len(samples) == 0 {
		return monitoringStatusResponse{
			Alert:   true,
			Source:  "network",
			Level:   "warning",
			Title:   "NO DATA",
			Message: "no samples",
		}
	}

	level := "ok"
	var messages []string

	for _, sample := range samples {
		if isIgnoredServiceTarget(sample) {
			continue
		}
		sampleLevel, message := sampleStatus(sample)
		if sampleLevel == "ok" {
			continue
		}
		if severityRank(sampleLevel) > severityRank(level) {
			level = sampleLevel
		}
		messages = append(messages, message)
	}

	if len(messages) == 0 {
		return monitoringStatusResponse{
			Alert:   false,
			Source:  "network",
			Level:   "ok",
			Title:   "NET OK",
			Message: "all probes healthy",
		}
	}
	if len(messages) > 2 {
		messages = append(messages[:2], fmt.Sprintf("%d more", len(messages)-2))
	}

	return monitoringStatusResponse{
		Alert:   true,
		Source:  "network",
		Level:   level,
		Title:   titleForLevel(level),
		Message: strings.Join(messages, ", "),
	}
}

func sampleStatus(sample model.Sample) (string, string) {
	switch sample.Type {
	case "dns":
		return dnsStatus(sample)
	case "http":
		return httpStatus(sample)
	}

	rtt := 0.0
	if sample.RTTAvgMs != nil {
		rtt = *sample.RTTAvgMs
	}

	lossPercent := 0.0
	if sample.LossPercent != nil {
		lossPercent = *sample.LossPercent
	}

	message := fmt.Sprintf("%s loss %.1f%%, rtt %.0fms", sample.Name, lossPercent, rtt)

	if sample.Error != "" {
		return "critical", sample.Name + " probe error"
	}
	if sample.Name == "gateway" && lossPercent > 0 {
		return "critical", message
	}
	if sample.Name != "gateway" && lossPercent >= 5 {
		return "critical", message
	}
	if sample.Name != "gateway" && lossPercent >= 1 {
		return "warning", message
	}
	if sample.Name != "gateway" && sample.RTTAvgMs != nil && *sample.RTTAvgMs >= 200 {
		return "critical", message
	}
	if sample.Name != "gateway" && sample.RTTAvgMs != nil && *sample.RTTAvgMs >= 100 {
		return "warning", message
	}

	return "ok", message
}

func dnsStatus(sample model.Sample) (string, string) {
	duration := 0.0
	if sample.DurationMs != nil {
		duration = *sample.DurationMs
	}
	message := fmt.Sprintf("%s dns %.0fms", sample.Name, duration)

	if sample.Error != "" || sample.OK != nil && !*sample.OK {
		return "warning", sample.Name + " dns failure"
	}
	if duration >= 1000 {
		return "critical", message
	}
	if duration >= 300 {
		return "warning", message
	}

	return "ok", message
}

func httpStatus(sample model.Sample) (string, string) {
	total := 0.0
	if sample.TotalMs != nil {
		total = *sample.TotalMs
	}
	label := sample.Name
	if sample.Group != "" {
		label = sample.Group
	}
	message := fmt.Sprintf("%s http total %.0fms", label, total)

	if sample.Error != "" || sample.OK != nil && !*sample.OK {
		if isTimeoutError(sample.Error) {
			return "warning", label + " timeout"
		}
		return "warning", label + " http failure"
	}
	if total >= 5000 {
		return "critical", message
	}
	if total >= 3000 {
		return "warning", message
	}

	return "ok", message
}

func sortSamplesByName(samples []model.Sample) {
	sort.SliceStable(samples, func(i, j int) bool {
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

func titleForLevel(level string) string {
	switch level {
	case "critical":
		return "NET DOWN"
	case "warning":
		return "NET SLOW"
	default:
		return "NET OK"
	}
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
