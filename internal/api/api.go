package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/model"
)

type Handler struct {
	state   *collector.State
	version string
}

func New(state *collector.State, version string) *Handler {
	return &Handler{
		state:   state,
		version: version,
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
	mux.HandleFunc("GET /api/monitoring/status", h.monitoringStatus)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "netwatch",
		"version": h.version,
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

	writeJSON(w, http.StatusOK, map[string]any{
		"name":    name,
		"range":   rangeValue,
		"samples": h.state.SeriesByType(sampleType, name, time.Now().Add(-duration)),
	})
}

func (h *Handler) monitoringStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildMonitoringStatus(h.state.LatestAll()))
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

	status := monitoringStatusResponse{
		Alert:   false,
		Source:  "network",
		Level:   "ok",
		Title:   "NET OK",
		Message: "targets normal",
	}

	for _, sample := range samples {
		level, message := sampleStatus(sample)
		if severityRank(level) > severityRank(status.Level) {
			status.Alert = level != "ok"
			status.Level = level
			status.Title = titleForLevel(level)
			status.Message = message
		}
	}

	return status
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
	message := fmt.Sprintf("%s http total %.0fms", sample.Name, total)

	if sample.Error != "" || sample.OK != nil && !*sample.OK {
		return "warning", sample.Name + " http failure"
	}
	if total >= 2000 {
		return "warning", message
	}

	return "ok", message
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
