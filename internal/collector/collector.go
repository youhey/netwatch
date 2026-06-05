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

type Collector struct {
	cfg     config.Config
	ping    PingProbe
	storage Storage
	state   *State
}

func New(cfg config.Config, ping PingProbe, storage Storage, state *State) *Collector {
	return &Collector{
		cfg:     cfg,
		ping:    ping,
		storage: storage,
		state:   state,
	}
}

func (c *Collector) Run(ctx context.Context) {
	c.collectOnce(ctx)

	ticker := time.NewTicker(time.Duration(c.cfg.PingIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectOnce(ctx)
		}
	}
}

func (c *Collector) collectOnce(ctx context.Context) {
	for _, target := range c.cfg.Targets {
		if target.Type != "ping" {
			continue
		}

		sample := c.measurePing(ctx, target)
		if err := c.storage.Append(sample); err != nil {
			log.Printf("append sample failed: name=%s target=%s error=%v", sample.Name, sample.Target, err)
			continue
		}
		c.state.Add(sample)
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

	sample.Sent = result.Sent
	sample.Received = result.Received
	sample.LossPercent = result.LossPercent
	sample.RTTMinMs = result.RTTMinMs
	sample.RTTAvgMs = result.RTTAvgMs
	sample.RTTMaxMs = result.RTTMaxMs
	if sample.Sent == 0 {
		sample.Sent = c.cfg.PingCount
		sample.LossPercent = 100
	}

	return sample
}
