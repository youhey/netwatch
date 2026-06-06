package api

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/youhey/netwatch/internal/config"
)

var (
	supportedRanges  = []string{"1h", "6h", "24h", "7d", "14d"}
	supportedBuckets = []string{"1m", "5m", "15m", "30m", "1h", "6h", "1d"}
)

type catalogTarget struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Target        string `json:"target,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	Group         string `json:"group,omitempty"`
	Category      string `json:"category,omitempty"`
	DisplayOrder  int    `json:"display_order,omitempty"`
	URL           string `json:"url,omitempty"`
	ExpectedBytes *int64 `json:"expected_bytes,omitempty"`
	Label         string `json:"label"`
}

type catalogServiceGroup struct {
	Group        string `json:"group"`
	DisplayName  string `json:"display_name"`
	Category     string `json:"category,omitempty"`
	DisplayOrder int    `json:"display_order,omitempty"`
	Label        string `json:"label"`
}

type chartSupportResponse struct {
	Ranges    []string       `json:"ranges"`
	Buckets   []string       `json:"buckets"`
	MaxPoints map[string]int `json:"max_points"`
}

type chartDefaultsResponse struct {
	Range     string `json:"range"`
	Bucket    string `json:"bucket"`
	MaxPoints int    `json:"max_points"`
}

type chartsOverviewResponse struct {
	GeneratedAt      time.Time       `json:"generated_at"`
	ActualRangeStart time.Time       `json:"actual_range_start"`
	ActualRangeEnd   time.Time       `json:"actual_range_end"`
	Timezone         string          `json:"timezone"`
	Range            string          `json:"range"`
	Bucket           string          `json:"bucket"`
	BucketSeconds    int             `json:"bucket_seconds"`
	MaxPoints        int             `json:"max_points"`
	Ping             []chartResponse `json:"ping"`
	HTTP             []chartResponse `json:"http"`
	Download         []chartResponse `json:"download"`
	ServiceGroups    []chartResponse `json:"service_groups"`
}

func (h *Handler) chartsCatalog(w http.ResponseWriter, r *http.Request) {
	pingTargets := make([]catalogTarget, 0)
	dnsTargets := make([]catalogTarget, 0)
	httpTargets := make([]catalogTarget, 0)
	downloadTargets := make([]catalogTarget, 0, len(h.downloadProbes))
	groups := make(map[string]catalogServiceGroup)

	for _, target := range h.targets {
		switch target.Type {
		case "ping":
			displayName := labelForTarget(target)
			pingTargets = append(pingTargets, catalogTarget{
				Name:         target.Name,
				DisplayName:  displayName,
				Target:       target.Target,
				DisplayOrder: target.DisplayOrder,
				Label:        displayName,
			})
		case "dns":
			displayName := labelForTarget(target)
			dnsTargets = append(dnsTargets, catalogTarget{
				Name:         target.Name,
				DisplayName:  displayName,
				Hostname:     target.Hostname,
				DisplayOrder: target.DisplayOrder,
				Label:        displayName,
			})
		case "http":
			if isIgnoredTargetName(target.Name) {
				continue
			}
			displayName := labelForTarget(target)
			httpTargets = append(httpTargets, catalogTarget{
				Name:         target.Name,
				DisplayName:  displayName,
				Group:        target.Group,
				Category:     target.Category,
				DisplayOrder: target.DisplayOrder,
				URL:          target.URL,
				Label:        displayName,
			})
			if target.Group != "" {
				if _, ok := groups[target.Group]; !ok {
					groups[target.Group] = catalogServiceGroup{
						Group:        target.Group,
						DisplayName:  labelForName(target.Group),
						Category:     target.Category,
						DisplayOrder: target.DisplayOrder,
						Label:        labelForName(target.Group),
					}
				} else if shouldReplaceDisplayOrder(groups[target.Group].DisplayOrder, target.DisplayOrder) {
					group := groups[target.Group]
					group.DisplayOrder = target.DisplayOrder
					groups[target.Group] = group
				}
			}
		}
	}
	for _, probe := range h.downloadProbes {
		expectedBytes := probe.ExpectedBytes
		displayName := labelForDownloadProbe(probe)
		downloadTargets = append(downloadTargets, catalogTarget{
			Name:          probe.Name,
			DisplayName:   displayName,
			URL:           probe.URL,
			ExpectedBytes: &expectedBytes,
			DisplayOrder:  probe.DisplayOrder,
			Label:         displayName,
		})
	}

	serviceGroups := make([]catalogServiceGroup, 0, len(groups))
	for _, group := range groups {
		serviceGroups = append(serviceGroups, group)
	}
	sortCatalogTargets(pingTargets)
	sortCatalogTargets(dnsTargets)
	sortCatalogTargets(httpTargets)
	sortCatalogTargets(downloadTargets)
	sort.SliceStable(serviceGroups, func(i, j int) bool {
		leftOrder := displayOrderRank(serviceGroups[i].DisplayOrder)
		rightOrder := displayOrderRank(serviceGroups[j].DisplayOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return serviceGroups[i].Group < serviceGroups[j].Group
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":   time.Now(),
		"timezone":       time.Now().Location().String(),
		"defaults":       chartDefaultsResponse{Range: "24h", Bucket: "5m", MaxPoints: defaultMaxPoints},
		"supported":      chartSupport(),
		"ping":           pingTargets,
		"dns":            dnsTargets,
		"http":           httpTargets,
		"download":       downloadTargets,
		"service_groups": serviceGroups,
	})
}

func (h *Handler) chartsOverview(w http.ResponseWriter, r *http.Request) {
	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	duration, err := parseRange(rangeValue)
	if err != nil {
		writeStructuredError(w, http.StatusBadRequest, "invalid_range", err.Error(), "range", nil)
		return
	}

	bucketValue := r.URL.Query().Get("bucket")
	if bucketValue == "" {
		bucketValue = "5m"
	}
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
	response := chartsOverviewResponse{
		GeneratedAt:      time.Now(),
		ActualRangeStart: start,
		ActualRangeEnd:   end,
		Timezone:         time.Now().Location().String(),
		Range:            rangeValue,
		Bucket:           bucketValue,
		BucketSeconds:    int(bucket.Seconds()),
		MaxPoints:        maxPoints,
	}

	for _, target := range orderedTargetsByType(h.targets, "ping") {
		samples := h.seriesByType("ping", target.Name, start)
		if len(samples) == 0 {
			continue
		}
		response.Ping = append(response.Ping, buildChartResponse("ping", rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
	}
	for _, target := range orderedTargetsByType(h.targets, "http") {
		if isIgnoredTargetName(target.Name) {
			continue
		}
		samples := h.seriesByType("http", target.Name, start)
		if len(samples) == 0 {
			continue
		}
		response.HTTP = append(response.HTTP, buildChartResponse("http", rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
	}
	for _, probe := range orderedDownloadProbes(h.downloadProbes) {
		samples := h.seriesByType("download", probe.Name, start)
		if len(samples) == 0 {
			continue
		}
		response.Download = append(response.Download, buildChartResponse("download", rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
	}
	for _, group := range []string{"youtube", "steam", "pcgame", "psn", "aws", "azure"} {
		samples := filterIgnoredServiceTargets(h.serviceSeries(group, "", start))
		if len(samples) == 0 {
			continue
		}
		response.ServiceGroups = append(response.ServiceGroups, buildServiceChartResponse(group, rangeValue, bucketValue, bucket, maxPoints, start, end, samples))
	}

	writeJSON(w, http.StatusOK, response)
}

func chartSupport() chartSupportResponse {
	return chartSupportResponse{
		Ranges:  supportedRanges,
		Buckets: supportedBuckets,
		MaxPoints: map[string]int{
			"min":     minMaxPoints,
			"max":     maxMaxPoints,
			"default": defaultMaxPoints,
		},
	}
}

func labelForTarget(target config.TargetConfig) string {
	if strings.TrimSpace(target.Label) != "" {
		return target.Label
	}
	if target.Group != "" && target.Name == "" {
		return labelForName(target.Group)
	}
	return labelForName(target.Name)
}

func labelForDownloadProbe(probe config.DownloadProbeConfig) string {
	if strings.TrimSpace(probe.Label) != "" {
		return probe.Label
	}
	return labelForName(probe.Name)
}

func labelForName(value string) string {
	parts := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		switch lower {
		case "dns", "http", "psn", "aws", "r2":
			parts[i] = strings.ToUpper(part)
			continue
		}
		if strings.HasSuffix(lower, "mb") && len(part) > 2 {
			parts[i] = part[:len(part)-2] + "MB"
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func sortCatalogTargets(targets []catalogTarget) {
	sort.SliceStable(targets, func(i, j int) bool {
		leftOrder := displayOrderRank(targets[i].DisplayOrder)
		rightOrder := displayOrderRank(targets[j].DisplayOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return targets[i].Name < targets[j].Name
	})
}

func orderedTargetsByType(targets []config.TargetConfig, targetType string) []config.TargetConfig {
	var filtered []config.TargetConfig
	for _, target := range targets {
		if target.Type != targetType {
			continue
		}
		filtered = append(filtered, target)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		leftOrder := displayOrderRank(filtered[i].DisplayOrder)
		rightOrder := displayOrderRank(filtered[j].DisplayOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return filtered[i].Name < filtered[j].Name
	})
	return filtered
}

func orderedDownloadProbes(probes []config.DownloadProbeConfig) []config.DownloadProbeConfig {
	ordered := append([]config.DownloadProbeConfig(nil), probes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftOrder := displayOrderRank(ordered[i].DisplayOrder)
		rightOrder := displayOrderRank(ordered[j].DisplayOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return ordered[i].Name < ordered[j].Name
	})
	return ordered
}

func displayOrderRank(value int) int {
	if value > 0 {
		return value
	}
	return math.MaxInt
}

func shouldReplaceDisplayOrder(current, candidate int) bool {
	if candidate <= 0 {
		return false
	}
	return current <= 0 || candidate < current
}
