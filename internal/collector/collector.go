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

type Collector struct {
	cfg     config.Config
	ping    PingProbe
	dns     DNSProbe
	http    HTTPProbe
	storage Storage
	state   *State
}

func New(cfg config.Config, ping PingProbe, dns DNSProbe, http HTTPProbe, storage Storage, state *State) *Collector {
	return &Collector{
		cfg:     cfg,
		ping:    ping,
		dns:     dns,
		http:    http,
		storage: storage,
		state:   state,
	}
}

func (c *Collector) Run(ctx context.Context) {
	c.collectType(ctx, "ping")
	c.collectType(ctx, "dns")
	c.collectType(ctx, "http")

	go c.runType(ctx, "dns", time.Duration(c.cfg.DNSIntervalSeconds)*time.Second)
	go c.runType(ctx, "http", time.Duration(c.cfg.HTTPIntervalSeconds)*time.Second)
	c.runType(ctx, "ping", time.Duration(c.cfg.PingIntervalSeconds)*time.Second)
}

func (c *Collector) runType(ctx context.Context, targetType string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectType(ctx, targetType)
		}
	}
}

func (c *Collector) collectType(ctx context.Context, targetType string) {
	for _, target := range c.cfg.Targets {
		if target.Type != targetType {
			continue
		}

		sample := c.measure(ctx, target)
		if err := c.storage.Append(sample); err != nil {
			log.Printf("append sample failed: type=%s name=%s error=%v", sample.Type, sample.Name, err)
			continue
		}
		c.state.Add(sample)
	}
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
		Target:    target.Target,
	}

	timeout := time.Duration(c.cfg.PingTimeoutSeconds) * time.Second
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
		Hostname:  target.Hostname,
	}

	timeout := time.Duration(c.cfg.DNSTimeoutSeconds) * time.Second
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
		URL:       target.URL,
		Method:    method,
	}

	timeout := time.Duration(c.cfg.HTTPTimeoutSeconds) * time.Second
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

	return sample
}

func boolPtr(value bool) *bool {
	return &value
}
