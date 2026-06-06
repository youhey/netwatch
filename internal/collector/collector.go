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
	Get(ctx context.Context, url string) (probe.HTTPResult, error)
}

type DownloadProbe interface {
	Get(ctx context.Context, url string, expectedBytes int64) (probe.DownloadResult, error)
}

type Collector struct {
	cfg      config.Config
	ping     PingProbe
	dns      DNSProbe
	http     HTTPProbe
	download DownloadProbe
	storage  Storage
	state    *State
}

func New(cfg config.Config, ping PingProbe, dns DNSProbe, http HTTPProbe, download DownloadProbe, storage Storage, state *State) *Collector {
	return &Collector{
		cfg:      cfg,
		ping:     ping,
		dns:      dns,
		http:     http,
		download: download,
		storage:  storage,
		state:    state,
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

		c.collectDownload(ctx, downloadProbe)
		nextRuns[key] = now.Add(time.Duration(downloadProbe.IntervalSeconds) * time.Second)
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

func (c *Collector) collectDownload(ctx context.Context, downloadProbe config.DownloadProbeConfig) {
	sample := c.measureDownload(ctx, downloadProbe)
	if err := c.storage.Append(sample); err != nil {
		log.Printf("append sample failed: type=%s name=%s error=%v", sample.Type, sample.Name, err)
		return
	}
	c.state.Add(sample)
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
		Timestamp: time.Now().Local(),
		Type:      target.Type,
		Name:      target.Name,
		Group:     target.Group,
		Category:  target.Category,
		Target:    target.Target,
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
		Timestamp: time.Now().Local(),
		Type:      target.Type,
		Name:      target.Name,
		Group:     target.Group,
		Category:  target.Category,
		Hostname:  target.Hostname,
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
		Timestamp: time.Now().Local(),
		Type:      target.Type,
		Name:      target.Name,
		Group:     target.Group,
		Category:  target.Category,
		URL:       target.URL,
		Method:    method,
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds(target)) * time.Second
	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.http.Get(httpCtx, target.URL)
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
