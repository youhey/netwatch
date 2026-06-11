package api

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
)

const aiExportFormat = "netwatch-ai-export-v1"

type exportStorageConfig struct {
	DataPath    string
	DataDir     string
	FilePattern string
}

type aiExportRequest struct {
	From          time.Time
	To            time.Time
	GeneratedAt   time.Time
	Filename      string
	Timezone      string
	RangeLabel    string
	ManifestRange aiExportRange `json:"range"`
}

type aiExportRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type aiExportManifest struct {
	Format          string        `json:"format"`
	GeneratedAt     time.Time     `json:"generated_at"`
	Range           aiExportRange `json:"range"`
	Timezone        string        `json:"timezone"`
	SampleCount     int           `json:"sample_count"`
	NetwatchVersion string        `json:"netwatch_version,omitempty"`
}

type aiExportTargets struct {
	Targets           []config.TargetConfig           `json:"targets"`
	DownloadProbes    []config.DownloadProbeConfig    `json:"download_probes"`
	RemoteSpeedProbes []config.RemoteSpeedProbeConfig `json:"remote_speed_probes"`
	StatusPages       []config.StatusPageConfig       `json:"status_pages"`
}

type aiExportThresholds struct {
	MonitoringThresholds config.MonitoringThresholds `json:"monitoring_thresholds"`
}

type aiExportSummary struct {
	Overall          aiExportSummaryOverall   `json:"overall"`
	Counts           aiExportSummaryCounts    `json:"counts"`
	Issues           aiExportSummaryIssues    `json:"issues"`
	ThroughputStatus aiExportThroughputStatus `json:"throughput_status"`
	Highlights       []string                 `json:"highlights"`
}

type aiExportSummaryOverall struct {
	NetworkWorstLevel        string `json:"network_worst_level"`
	ServiceHealthWorstLevel  string `json:"service_health_worst_level"`
	ProviderStatusWorstLevel string `json:"provider_status_worst_level"`
	SpeedprobeWorstLevel     string `json:"speedprobe_worst_level"`
}

type aiExportSummaryCounts struct {
	Samples    int `json:"samples"`
	Ping       int `json:"ping"`
	DNS        int `json:"dns"`
	HTTP       int `json:"http"`
	Download   int `json:"download"`
	Speedprobe int `json:"speedprobe"`
	StatusPage int `json:"status_page"`
}

type aiExportSummaryIssues struct {
	Network        int `json:"network"`
	ServiceHealth  int `json:"service_health"`
	ProviderStatus int `json:"provider_status"`
	Speedprobe     int `json:"speedprobe"`
}

type aiExportThroughputStatus struct {
	WorstLevel string   `json:"worst_level"`
	IssueCount int      `json:"issue_count"`
	Sources    []string `json:"sources"`
}

type aiExportAggregate struct {
	counts             aiExportSummaryCounts
	networkWorst       string
	serviceWorst       string
	providerWorst      string
	networkIssues      int
	serviceIssues      int
	providerIssues     int
	downloadSlow       int
	downloadFailure    int
	gatewayLoss        int
	gatewayRTTHigh     int
	externalIssues     int
	dnsIssues          int
	serviceGroupFailed map[string]int
	speedprobeIssues   int
	speedprobeWorst    string
	throughputIssues   int
	throughputWorst    string
	throughputSources  map[string]struct{}
}

func (h *Handler) aiExport(w http.ResponseWriter, r *http.Request) {
	req, err := parseAIExportRequest(r, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_range",
			"message": err.Error(),
		})
		return
	}

	path, err := h.buildAIExportZip(req)
	if err != nil {
		log.Printf("AI export generation failed: error=%v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   "export_generation_failed",
			"message": "could not create AI analysis export",
		})
		return
	}
	defer func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove AI export temp file failed: path=%s error=%v", path, err)
		}
	}()

	f, err := os.Open(path)
	if err != nil {
		log.Printf("open AI export failed: path=%s error=%v", path, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   "export_open_failed",
			"message": "could not open AI analysis export",
		})
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		log.Printf("stat AI export failed: path=%s error=%v", path, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   "export_stat_failed",
			"message": "could not read AI analysis export",
		})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, req.Filename))
	http.ServeContent(w, r, req.Filename, stat.ModTime(), f)
}

func parseAIExportRequest(r *http.Request, now time.Time) (aiExportRequest, error) {
	location := now.Location()
	fromValue := strings.TrimSpace(r.URL.Query().Get("from"))
	toValue := strings.TrimSpace(r.URL.Query().Get("to"))

	var from, to time.Time
	rangeLabel := strings.TrimSpace(r.URL.Query().Get("range"))
	if fromValue != "" || toValue != "" {
		if fromValue == "" || toValue == "" {
			return aiExportRequest{}, fmt.Errorf("from and to must be specified together")
		}
		var err error
		from, err = parseAIExportDate(fromValue, location)
		if err != nil {
			return aiExportRequest{}, fmt.Errorf("from must be YYYY-MM-DD")
		}
		to, err = parseAIExportDate(toValue, location)
		if err != nil {
			return aiExportRequest{}, fmt.Errorf("to must be YYYY-MM-DD")
		}
		rangeLabel = fromValue + "_" + toValue
	} else {
		if rangeLabel == "" {
			rangeLabel = "7d"
		}
		days, ok := map[string]int{"1d": 1, "7d": 7, "30d": 30}[rangeLabel]
		if !ok {
			return aiExportRequest{}, fmt.Errorf("range must be one of 1d, 7d, 30d")
		}
		to = now
		from = to.AddDate(0, 0, -days)
	}

	if !to.After(from) {
		return aiExportRequest{}, fmt.Errorf("to must be after from")
	}

	return aiExportRequest{
		From:        from,
		To:          to,
		GeneratedAt: now,
		Filename:    fmt.Sprintf("netwatch-export-%s_%s.zip", from.Format("2006-01-02"), to.Format("2006-01-02")),
		Timezone:    location.String(),
		RangeLabel:  rangeLabel,
		ManifestRange: aiExportRange{
			From: from,
			To:   to,
		},
	}, nil
}

func parseAIExportDate(value string, location *time.Location) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, err
	}
	return date, nil
}

func (h *Handler) buildAIExportZip(req aiExportRequest) (string, error) {
	exportDir := h.exportDir()
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", err
	}
	cleanupAIExports(exportDir, req.GeneratedAt)

	tmp, err := os.CreateTemp(exportDir, "netwatch-export-*.zip")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	removeOnError := true
	defer func() {
		_ = tmp.Close()
		if removeOnError {
			_ = os.Remove(path)
		}
	}()

	zw := zip.NewWriter(tmp)
	samplesWriter, err := zw.Create("samples.jsonl")
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	agg := newAIExportAggregate()
	if err := h.writeAIExportSamples(samplesWriter, req, agg); err != nil {
		_ = zw.Close()
		return "", err
	}

	files := map[string]any{
		"manifest.json": aiExportManifest{
			Format:          aiExportFormat,
			GeneratedAt:     req.GeneratedAt,
			Range:           req.ManifestRange,
			Timezone:        req.Timezone,
			SampleCount:     agg.counts.Samples,
			NetwatchVersion: h.version,
		},
		"targets.json": aiExportTargets{
			Targets:           h.targets,
			DownloadProbes:    h.downloadProbes,
			RemoteSpeedProbes: h.remoteSpeedProbes,
			StatusPages:       h.statusPages,
		},
		"thresholds.json": aiExportThresholds{
			MonitoringThresholds: h.thresholds,
		},
		"summary.json": agg.summary(),
	}
	for _, name := range []string{"manifest.json", "targets.json", "thresholds.json", "summary.json"} {
		if err := writeAIExportJSON(zw, name, files[name]); err != nil {
			_ = zw.Close()
			return "", err
		}
	}
	if err := writeAIExportText(zw, "README.md", aiExportReadme(req)); err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := writeAIExportText(zw, "analysis-prompt.md", aiExportPrompt(req)); err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	removeOnError = false
	return path, nil
}

func (h *Handler) writeAIExportSamples(w io.Writer, req aiExportRequest, agg *aiExportAggregate) error {
	paths, err := h.aiExportDataFiles(req)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := h.writeAIExportSamplesFromFile(w, path, req, agg); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) writeAIExportSamplesFromFile(w io.Writer, path string, req aiExportRequest, agg *aiExportAggregate) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var sample model.Sample
		if err := json.Unmarshal(line, &sample); err != nil {
			log.Printf("skip invalid JSONL line during AI export: path=%s line=%d error=%v", path, lineNumber, err)
			continue
		}
		if sample.Type == "" && sample.Kind != "" {
			sample.Type = sample.Kind
		}
		if sample.Timestamp.Before(req.From) || !sample.Timestamp.Before(req.To) {
			continue
		}
		sample = h.applyDisplayMetadata([]model.Sample{sample})[0]
		if sample.Kind == "" {
			sample.Kind = sample.Type
		}
		agg.add(sample, h.thresholds)
		b, err := json.Marshal(sample)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (h *Handler) aiExportDataFiles(req aiExportRequest) ([]string, error) {
	if h.exportStorage.DataDir == "" {
		if h.exportStorage.DataPath == "" {
			return nil, fmt.Errorf("export storage is not configured")
		}
		return []string{h.exportStorage.DataPath}, nil
	}

	pattern := h.exportStorage.FilePattern
	if pattern == "" {
		pattern = "samples-%Y-%m-%d.jsonl"
	}
	var paths []string
	for day := startOfLocalDay(req.From); !day.After(startOfLocalDay(req.To)); day = day.AddDate(0, 0, 1) {
		paths = append(paths, filepath.Join(h.exportStorage.DataDir, strings.ReplaceAll(pattern, "%Y-%m-%d", day.Format("2006-01-02"))))
	}
	sort.Strings(paths)
	return paths, nil
}

func startOfLocalDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func (h *Handler) exportDir() string {
	if h.exportStorage.DataDir != "" {
		return filepath.Join(h.exportStorage.DataDir, "exports")
	}
	if h.exportStorage.DataPath != "" {
		return filepath.Join(filepath.Dir(h.exportStorage.DataPath), "exports")
	}
	return filepath.Join(os.TempDir(), "netwatch-exports")
}

func cleanupAIExports(dir string, now time.Time) {
	paths, err := filepath.Glob(filepath.Join(dir, "netwatch-export-*.zip"))
	if err != nil {
		log.Printf("list AI export files failed: dir=%s error=%v", dir, err)
		return
	}
	cutoff := now.Add(-24 * time.Hour)
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil {
			continue
		}
		if stat.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil {
			log.Printf("remove old AI export failed: path=%s error=%v", path, err)
		}
	}
}

func newAIExportAggregate() *aiExportAggregate {
	return &aiExportAggregate{
		networkWorst:       "ok",
		serviceWorst:       "ok",
		providerWorst:      "ok",
		speedprobeWorst:    "ok",
		throughputWorst:    "ok",
		serviceGroupFailed: make(map[string]int),
		throughputSources:  make(map[string]struct{}),
	}
}

func (a *aiExportAggregate) add(sample model.Sample, thresholds config.MonitoringThresholds) {
	a.counts.Samples++
	switch sample.Type {
	case "ping":
		a.counts.Ping++
		a.addNetworkReasons(pingReasons(sample, thresholds.Ping))
	case "dns":
		a.counts.DNS++
		a.addNetworkReasons(dnsReasons(sample, thresholds.DNS))
	case "download":
		a.counts.Download++
		a.addThroughputSample(sample, thresholds)
	case "http":
		a.counts.HTTP++
		a.addServiceSample(sample, thresholds)
	case "speedprobe":
		a.counts.Speedprobe++
		a.addThroughputSample(sample, thresholds)
		a.addSpeedprobeSample(sample)
	case "status_page":
		a.counts.StatusPage++
		a.addProviderSample(sample)
	}
}

func (a *aiExportAggregate) addNetworkReasons(reasons []monitoringReason) {
	for _, reason := range reasons {
		a.networkIssues++
		a.networkWorst = worseAIExportLevel(a.networkWorst, reason.Level)
		switch reason.Code {
		case "gateway_loss":
			a.gatewayLoss++
		case "gateway_rtt_high":
			a.gatewayRTTHigh++
		case "packet_loss", "external_rtt_high":
			a.externalIssues++
		case "dns_failure", "dns_slow":
			a.dnsIssues++
		}
	}
}

func (a *aiExportAggregate) addThroughputSample(sample model.Sample, thresholds config.MonitoringThresholds) {
	sourceName, _, _ := throughputSourceMetadata(sample)
	if sourceName == "" {
		return
	}
	a.throughputSources[sourceName] = struct{}{}
	level, reason := throughputProbeLevel(sample, thresholds)
	if level == "ok" {
		return
	}
	a.throughputIssues++
	a.throughputWorst = worseAIExportLevel(a.throughputWorst, level)
	switch reason {
	case "download_slow":
		a.downloadSlow++
	case "download_failure":
		a.downloadFailure++
	}
}

func (a *aiExportAggregate) addServiceSample(sample model.Sample, thresholds config.MonitoringThresholds) {
	if !isServiceHealthSample(sample) {
		return
	}
	level := "ok"
	if sample.OK == nil {
		level = "unknown"
	} else if !*sample.OK || sample.Error != "" {
		level = "warning"
		a.serviceGroupFailed[sample.Group]++
	} else if sample.TotalMs != nil {
		if *sample.TotalMs >= thresholds.HTTP.TotalMs.Critical {
			level = "critical"
		} else if *sample.TotalMs >= thresholds.HTTP.TotalMs.Warning {
			level = "warning"
		}
	}
	if level == "ok" {
		return
	}
	a.serviceIssues++
	a.serviceWorst = worseAIExportLevel(a.serviceWorst, level)
}

func (a *aiExportAggregate) addProviderSample(sample model.Sample) {
	level := statusPageLevel(sample)
	a.providerWorst = worseAIExportLevel(a.providerWorst, level)
	switch level {
	case "warning", "critical":
		a.providerIssues++
	}
}

func (a *aiExportAggregate) addSpeedprobeSample(sample model.Sample) {
	level := "ok"
	if strings.EqualFold(sample.Status, "unknown") || sample.OK == nil {
		level = "unknown"
	} else if !*sample.OK || sample.Error != "" {
		level = "warning"
	}
	if level == "ok" {
		return
	}
	a.speedprobeIssues++
	a.speedprobeWorst = worseAIExportLevel(a.speedprobeWorst, level)
}

func (a *aiExportAggregate) summary() aiExportSummary {
	for _, failed := range a.serviceGroupFailed {
		if failed > 1 {
			a.serviceWorst = worseAIExportLevel(a.serviceWorst, "critical")
		}
	}
	return aiExportSummary{
		Overall: aiExportSummaryOverall{
			NetworkWorstLevel:        a.networkWorst,
			ServiceHealthWorstLevel:  a.serviceWorst,
			ProviderStatusWorstLevel: a.providerWorst,
			SpeedprobeWorstLevel:     a.speedprobeWorst,
		},
		Counts: a.counts,
		Issues: aiExportSummaryIssues{
			Network:        a.networkIssues,
			ServiceHealth:  a.serviceIssues,
			ProviderStatus: a.providerIssues,
			Speedprobe:     a.speedprobeIssues,
		},
		ThroughputStatus: aiExportThroughputStatus{
			WorstLevel: a.throughputWorst,
			IssueCount: a.throughputIssues,
			Sources:    sortedStringSet(a.throughputSources),
		},
		Highlights: a.highlights(),
	}
}

func (a *aiExportAggregate) highlights() []string {
	highlights := make([]string, 0, 6)
	if a.downloadSlow > 0 {
		highlights = append(highlights, fmt.Sprintf("Download throughput dropped below throughput threshold %d times.", a.downloadSlow))
	}
	if a.downloadFailure > 0 {
		highlights = append(highlights, fmt.Sprintf("Download probe failed %d times in Throughput Status.", a.downloadFailure))
	}
	if a.gatewayLoss == 0 && a.gatewayRTTHigh == 0 {
		highlights = append(highlights, "Gateway latency and packet loss were stable throughout the range.")
	}
	if a.externalIssues > 0 {
		highlights = append(highlights, fmt.Sprintf("External connectivity issues were observed %d times.", a.externalIssues))
	}
	if a.dnsIssues > 0 {
		highlights = append(highlights, fmt.Sprintf("DNS issues were observed %d times.", a.dnsIssues))
	}
	if a.serviceIssues > 0 {
		highlights = append(highlights, "HTTP service probe issues were observed but do not affect Core Network Status.")
	}
	if a.providerIssues > 0 {
		highlights = append(highlights, "Provider status page issues were observed separately from local network health.")
	}
	if a.counts.Speedprobe > 0 {
		highlights = append(highlights, "Remote speedprobe throughput samples are included for WAN performance analysis.")
	}
	if len(highlights) == 0 {
		highlights = append(highlights, "No issues were detected in the exported range.")
	}
	return highlights
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func worseAIExportLevel(left, right string) string {
	if aiExportLevelRank(right) > aiExportLevelRank(left) {
		return right
	}
	return left
}

func aiExportLevelRank(level string) int {
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

func writeAIExportJSON(zw *zip.Writer, name string, value any) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeAIExportText(zw *zip.Writer, name, value string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, value)
	return err
}

func aiExportReadme(req aiExportRequest) string {
	return fmt.Sprintf(`# Netwatch AI Export

This archive contains observation data exported from netwatch for AI-assisted analysis.

## Range

- From: %s
- To: %s
- Timezone: %s

## Files

- manifest.json: export metadata
- targets.json: monitoring target snapshot without secrets
- thresholds.json: monitoring thresholds at export time
- summary.json: machine-generated summary for first-pass analysis
- samples.jsonl: raw samples in the requested range
- analysis-prompt.md: prompt text for AI-assisted analysis

## Status scopes

- Core Network Status: Gateway / External / DNS probes. This represents baseline local network health.
- Throughput Status: Download throughput observations from legacy download_probes and netwatch-speedprobe. These are used for ISP comparison, time-of-day congestion analysis, and WAN throughput diagnostics. They do not affect Core Network Status.
- Service Health: HTTP endpoint probes such as GitHub, ChatGPT, Docker, YouTube, and others. These are supplemental observations and may be affected by provider-side issues.
- Provider Status: Official provider status pages such as GitHub Status, OpenAI Status, Cloudflare Status, and Laravel Cloud Status.
- Speedprobe: Remote throughput measurements performed by netwatch-speedprobe on a stronger observer node such as scum. These are part of Throughput Status.
`, req.From.Format(time.RFC3339), req.To.Format(time.RFC3339), req.Timezone)
}

func aiExportPrompt(req aiExportRequest) string {
	return fmt.Sprintf(`# Netwatch Analysis Request

このZIPは、自宅ネットワーク監視ツール netwatch の観測データです。

対象期間:
- From: %s
- To: %s

以下を分析してください。

1. 自宅ネットワーク品質の傾向
2. 異常が発生した時間帯
3. Gateway / External / DNS と Throughput Status の相関
4. HTTP Service issue と Core Network issue の切り分け
5. Provider Status issue との関係
6. Throughput Status の 1MB / 10MB / 100MB の速度推移を時間帯別に分析
7. Core Network Status が正常な時間帯で Throughput だけ低下しているケース
8. ISP / PPPoE / マンション共有回線の混雑が疑われる時間帯
9. speedprobe と legacy download_probes の差
10. 改善候補
11. 追加で監視した方がよい項目
`, req.From.Format(time.RFC3339), req.To.Format(time.RFC3339))
}
