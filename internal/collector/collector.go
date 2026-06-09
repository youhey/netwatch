package collector

import (
	"context"
	"log"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
	"github.com/youhey/netwatch/internal/probe"
)

type Storage interface {
	Append(sample model.Sample) error
}

type PingProbe interface {
	Ping(ctx context.Context, target string, count int) (probe.PingResult, error)
}

type DNSProbe interface {
	Lookup(ctx context.Context, hostname string) (probe.DNSResult, error)
}

type HTTPProbe interface {
	Get(ctx context.Context, url string, expectedStatuses []int) (probe.HTTPResult, error)
}

type DownloadProbe interface {
	Get(ctx context.Context, url string, expectedBytes int64) (probe.DownloadResult, error)
}

type Collector struct {
	cfg             config.Config
	ping            PingProbe
	dns             DNSProbe
	http            HTTPProbe
	download        DownloadProbe
	storage         Storage
	state           *State
	downloadRetries map[string]*downloadRetryRuntime
}

func New(cfg config.Config, ping PingProbe, dns DNSProbe, http HTTPProbe, download DownloadProbe, storage Storage, state *State) *Collector {
	return &Collector{
		cfg:             cfg,
		ping:            ping,
		dns:             dns,
		http:            http,
		download:        download,
		storage:         storage,
		state:           state,
		downloadRetries: make(map[string]*downloadRetryRuntime),
	}
}

func (c *Collector) Run(ctx context.Context) {
	downloadProbes := c.cfg.EnabledDownloadProbes()
	nextRuns := make(map[string]time.Time, len(c.cfg.Targets)+len(downloadProbes))
	now := time.Now()
	for _, target := range c.cfg.Targets {
		nextRuns["target:"+target.Name] = now
	}
	for _, downloadProbe := range downloadProbes {
		nextRuns["download:"+downloadProbe.Name] = now
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		c.collectDue(ctx, nextRuns)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Collector) collectDue(ctx context.Context, nextRuns map[string]time.Time) {
	now := time.Now()
	for _, target := range c.cfg.Targets {
		key := "target:" + target.Name
		if now.Before(nextRuns[key]) {
			continue
		}

		c.collectTarget(ctx, target)
		nextRuns[key] = now.Add(time.Duration(c.cfg.IntervalSeconds(target)) * time.Second)
	}
	for _, downloadProbe := range c.cfg.EnabledDownloadProbes() {
		key := "download:" + downloadProbe.Name
		if now.Before(nextRuns[key]) {
			continue
		}

		nextRuns[key] = c.collectDownload(ctx, downloadProbe, now)
	}
}

func (c *Collector) collectTarget(ctx context.Context, target config.TargetConfig) {
	sample := c.measure(ctx, target)
	if err := c.storage.Append(sample); err != nil {
		log.Printf("append sample failed: type=%s name=%s error=%v", sample.Type, sample.Name, err)
		return
	}
	c.state.Add(sample)
}

func (c *Collector) collectDownload(ctx context.Context, downloadProbe config.DownloadProbeConfig, checkedAt time.Time) time.Time {
	sample := c.measureDownload(ctx, downloadProbe)
	nextCheckAt := c.updateDownloadRetry(downloadProbe, &sample, checkedAt)
	if err := c.storage.Append(sample); err != nil {
		log.Printf("append sample failed: type=%s name=%s error=%v", sample.Type, sample.Name, err)
		return nextCheckAt
	}
	c.state.Add(sample)
	return nextCheckAt
}

func (c *Collector) measure(ctx context.Context, target config.TargetConfig) model.Sample {
	switch target.Type {
	case "ping":
		return c.measurePing(ctx, target)
	case "dns":
		return c.measureDNS(ctx, target)
	case "http":
		return c.measureHTTP(ctx, target)
	default:
		return model.Sample{
			Timestamp: time.Now().Local(),
			Type:      target.Type,
			Name:      target.Name,
			OK:        boolPtr(false),
			Error:     "unsupported target type",
		}
	}
}

func (c *Collector) measurePing(ctx context.Context, target config.TargetConfig) model.Sample {
	sample := model.Sample{
		Timestamp:    time.Now().Local(),
		Type:         target.Type,
		Name:         target.Name,
		Group:        target.Group,
		Category:     target.Category,
		DisplayName:  target.Label,
		DisplayOrder: target.DisplayOrder,
		Target:       target.Target,
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds(target)) * time.Second
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.ping.Ping(pingCtx, target.Target, c.cfg.PingCount)
	if err != nil {
		sample.Error = err.Error()
	}

	lossPercent := result.LossPercent
	sample.Sent = result.Sent
	sample.Received = result.Received
	sample.LossPercent = &lossPercent
	sample.RTTMinMs = result.RTTMinMs
	sample.RTTAvgMs = result.RTTAvgMs
	sample.RTTMaxMs = result.RTTMaxMs
	if sample.Sent == 0 {
		sample.Sent = c.cfg.PingCount
		lossPercent = 100
		sample.LossPercent = &lossPercent
	}
	sample.OK = boolPtr(sample.Error == "" && sample.Received > 0)

	return sample
}

func (c *Collector) measureDNS(ctx context.Context, target config.TargetConfig) model.Sample {
	sample := model.Sample{
		Timestamp:    time.Now().Local(),
		Type:         target.Type,
		Name:         target.Name,
		Group:        target.Group,
		Category:     target.Category,
		DisplayName:  target.Label,
		DisplayOrder: target.DisplayOrder,
		Hostname:     target.Hostname,
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds(target)) * time.Second
	dnsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.dns.Lookup(dnsCtx, target.Hostname)
	if err != nil {
		sample.Error = err.Error()
	}

	sample.OK = boolPtr(result.OK)
	sample.DurationMs = &result.DurationMs
	sample.ResolvedIPs = result.ResolvedIPs

	return sample
}

func (c *Collector) measureHTTP(ctx context.Context, target config.TargetConfig) model.Sample {
	method := target.Method
	if method == "" {
		method = "GET"
	}

	sample := model.Sample{
		Timestamp:    time.Now().Local(),
		Type:         target.Type,
		Name:         target.Name,
		Group:        target.Group,
		Category:     target.Category,
		DisplayName:  target.Label,
		DisplayOrder: target.DisplayOrder,
		URL:          target.URL,
		Method:       method,
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds(target)) * time.Second
	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.http.Get(httpCtx, target.URL, target.ExpectedStatuses)
	if err != nil {
		sample.Error = err.Error()
	}

	sample.OK = boolPtr(result.OK && err == nil)
	sample.HTTPStatus = result.HTTPStatus
	sample.DNSMs = result.DNSMs
	sample.ConnectMs = result.ConnectMs
	sample.TLSMs = result.TLSMs
	sample.TTFBMs = result.TTFBMs
	sample.TotalMs = &result.TotalMs
	sample.RemoteAddr = result.RemoteAddr
	sample.ContentLength = result.ContentLength
	sample.ContentLengthRead = &result.ContentLengthRead
	sample.BodyTruncated = &result.BodyTruncated

	return sample
}

func (c *Collector) measureDownload(ctx context.Context, downloadProbe config.DownloadProbeConfig) model.Sample {
	sample := model.Sample{
		Timestamp:     time.Now().Local(),
		Type:          "download",
		Name:          downloadProbe.Name,
		DisplayName:   downloadProbe.Label,
		DisplayOrder:  downloadProbe.DisplayOrder,
		URL:           downloadProbe.URL,
		ExpectedBytes: positiveInt64Ptr(downloadProbe.ExpectedBytes),
	}

	timeout := time.Duration(downloadProbe.TimeoutSeconds) * time.Second
	downloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.download.Get(downloadCtx, downloadProbe.URL, downloadProbe.ExpectedBytes)
	if err != nil {
		sample.Error = err.Error()
	}

	sample.OK = boolPtr(result.OK && err == nil)
	sample.HTTPStatus = result.HTTPStatus
	sample.DownloadedBytes = int64Ptr(result.DownloadedBytes)
	sample.DurationMs = &result.DurationMs
	sample.BytesPerSec = &result.BytesPerSec
	sample.Mbps = &result.Mbps

	return sample
}

const (
	downloadRetryStateNormal     = "normal"
	downloadRetryStateDegraded   = "degraded"
	downloadRetryStateRecovering = "recovering"
)

type downloadRetryRuntime struct {
	State                string
	Attempt              int
	RecoverySuccessCount int
	NextCheckAt          time.Time
	LastResultLevel      string
}

func (c *Collector) updateDownloadRetry(downloadProbe config.DownloadProbeConfig, sample *model.Sample, checkedAt time.Time) time.Time {
	retry := downloadProbe.EffectiveRetryOnAlert()
	if !retry.Enabled {
		return checkedAt.Add(time.Duration(downloadProbe.IntervalSeconds) * time.Second)
	}

	runtime := c.downloadRetryState(downloadProbe.Name)
	resultLevel := c.downloadResultLevel(*sample)
	alert := resultLevel == "warning" || resultLevel == "critical"

	switch runtime.State {
	case downloadRetryStateDegraded, downloadRetryStateRecovering:
		c.updateActiveDownloadRetry(runtime, retry, alert, checkedAt, downloadProbe.IntervalSeconds)
	default:
		c.updateNormalDownloadRetry(runtime, retry, alert, checkedAt, downloadProbe.IntervalSeconds)
	}
	runtime.LastResultLevel = resultLevel
	c.applyDownloadRetryMetadata(sample, runtime)
	return runtime.NextCheckAt
}

func (c *Collector) downloadRetryState(name string) *downloadRetryRuntime {
	if c.downloadRetries == nil {
		c.downloadRetries = make(map[string]*downloadRetryRuntime)
	}
	runtime, ok := c.downloadRetries[name]
	if !ok {
		runtime = &downloadRetryRuntime{State: downloadRetryStateNormal}
		c.downloadRetries[name] = runtime
	}
	return runtime
}

func (c *Collector) updateNormalDownloadRetry(runtime *downloadRetryRuntime, retry config.RetryOnAlertConfig, alert bool, checkedAt time.Time, normalIntervalSeconds int) {
	if !alert {
		runtime.State = downloadRetryStateNormal
		runtime.Attempt = 0
		runtime.RecoverySuccessCount = 0
		runtime.NextCheckAt = checkedAt.Add(time.Duration(normalIntervalSeconds) * time.Second)
		return
	}
	runtime.State = downloadRetryStateDegraded
	runtime.Attempt = 0
	runtime.RecoverySuccessCount = 0
	runtime.NextCheckAt = checkedAt.Add(time.Duration(retryIntervalSeconds(retry, runtime.Attempt)) * time.Second)
}

func (c *Collector) updateActiveDownloadRetry(runtime *downloadRetryRuntime, retry config.RetryOnAlertConfig, alert bool, checkedAt time.Time, normalIntervalSeconds int) {
	if alert {
		runtime.State = downloadRetryStateDegraded
		runtime.RecoverySuccessCount = 0
		runtime.Attempt++
		runtime.NextCheckAt = checkedAt.Add(time.Duration(retryIntervalSeconds(retry, runtime.Attempt)) * time.Second)
		return
	}

	runtime.State = downloadRetryStateRecovering
	runtime.RecoverySuccessCount++
	if runtime.RecoverySuccessCount >= retry.RecoverySuccessCount {
		runtime.State = downloadRetryStateNormal
		runtime.Attempt = 0
		runtime.RecoverySuccessCount = 0
		runtime.NextCheckAt = checkedAt.Add(time.Duration(normalIntervalSeconds) * time.Second)
		return
	}
	runtime.NextCheckAt = checkedAt.Add(time.Duration(retryIntervalSeconds(retry, runtime.Attempt)) * time.Second)
}

func retryIntervalSeconds(retry config.RetryOnAlertConfig, attempt int) int {
	if len(retry.IntervalsSeconds) == 0 {
		return 1
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(retry.IntervalsSeconds) {
		attempt = len(retry.IntervalsSeconds) - 1
	}
	return retry.IntervalsSeconds[attempt]
}

func (c *Collector) downloadResultLevel(sample model.Sample) string {
	if sample.Error != "" || sample.OK != nil && !*sample.OK {
		return "warning"
	}
	if sample.Mbps == nil {
		return "ok"
	}
	threshold, ok := c.cfg.MonitoringThresholds.Download[sample.Name+"_mbps"]
	if !ok {
		return "ok"
	}
	if *sample.Mbps < threshold.Critical {
		return "critical"
	}
	if *sample.Mbps < threshold.Warning {
		return "warning"
	}
	return "ok"
}

func (c *Collector) applyDownloadRetryMetadata(sample *model.Sample, runtime *downloadRetryRuntime) {
	attempt := runtime.Attempt
	recoverySuccessCount := runtime.RecoverySuccessCount
	nextCheckAt := runtime.NextCheckAt
	sample.RetryState = runtime.State
	sample.RetryAttempt = &attempt
	sample.RecoverySuccessCount = &recoverySuccessCount
	sample.NextCheckAt = &nextCheckAt
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func positiveInt64Ptr(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}
