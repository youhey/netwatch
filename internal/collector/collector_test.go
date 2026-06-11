package collector

import (
	"context"
	"testing"
	"time"

	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/model"
	"github.com/youhey/netwatch/internal/probe"
	"github.com/youhey/netwatch/internal/speedprobe"
)

type fakeHTTPProbe struct {
	deadline         time.Time
	expectedStatuses []int
}

type fakeDownloadProbe struct {
	deadline time.Time
}

type fakeStatusPageProbe struct {
	deadline            time.Time
	importantComponents []string
}

type fakeSpeedprobeClient struct {
	deadline time.Time
	timeout  time.Duration
	latest   speedprobe.LatestResponse
	err      error
}

type fakeStorage struct {
	samples []model.Sample
}

func (p *fakeHTTPProbe) Get(ctx context.Context, url string, expectedStatuses []int) (probe.HTTPResult, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		p.deadline = deadline
	}
	p.expectedStatuses = append([]int(nil), expectedStatuses...)
	status := 200
	return probe.HTTPResult{
		OK:         true,
		HTTPStatus: &status,
		TotalMs:    12.3,
	}, nil
}

func (p *fakeDownloadProbe) Get(ctx context.Context, url string, expectedBytes int64) (probe.DownloadResult, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		p.deadline = deadline
	}
	return probe.DownloadResult{
		OK:              true,
		DownloadedBytes: expectedBytes,
		DurationMs:      1000,
		BytesPerSec:     float64(expectedBytes),
		Mbps:            float64(expectedBytes) * 8 / 1_000_000,
	}, nil
}

func (p *fakeStatusPageProbe) Get(ctx context.Context, url string, importantComponents []string) (probe.StatusPageResult, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		p.deadline = deadline
	}
	p.importantComponents = append([]string(nil), importantComponents...)
	return probe.StatusPageResult{
		OK:          true,
		Level:       "ok",
		Indicator:   "none",
		Description: "All Systems Operational",
		DurationMs:  123,
	}, nil
}

func (c *fakeSpeedprobeClient) Latest(ctx context.Context, url string, timeout time.Duration) (speedprobe.LatestResponse, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		c.deadline = deadline
	}
	c.timeout = timeout
	if c.err != nil {
		return speedprobe.LatestResponse{}, c.err
	}
	return c.latest, nil
}

func (s *fakeStorage) Append(sample model.Sample) error {
	s.samples = append(s.samples, sample)
	return nil
}

func TestMeasureHTTPUsesTargetMetadataAndTimeout(t *testing.T) {
	httpProbe := &fakeHTTPProbe{}
	cfg := config.Default()
	target := config.TargetConfig{
		Name:           "youtube_home",
		Type:           "http",
		Group:          "youtube",
		Category:       "service",
		Label:          "YouTube Home",
		URL:            "https://www.youtube.com/",
		TimeoutSeconds: 3,
		DisplayOrder:   10,
		ExpectedStatuses: []int{
			200,
			401,
		},
	}
	collector := New(cfg, nil, nil, httpProbe, nil, nil, NewState())

	before := time.Now()
	sample := collector.measureHTTP(context.Background(), target)
	remaining := time.Until(httpProbe.deadline)

	if sample.Group != "youtube" || sample.Category != "service" || sample.DisplayName != "YouTube Home" || sample.DisplayOrder != 10 {
		t.Fatalf("sample = %+v, want group/category metadata", sample)
	}
	if sample.TotalMs == nil || *sample.TotalMs != 12.3 {
		t.Fatalf("TotalMs = %v, want 12.3", sample.TotalMs)
	}
	if len(httpProbe.expectedStatuses) != 2 || httpProbe.expectedStatuses[0] != 200 || httpProbe.expectedStatuses[1] != 401 {
		t.Fatalf("expectedStatuses = %+v, want target expected_statuses", httpProbe.expectedStatuses)
	}
	if httpProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want target timeout around 3s", remaining)
	}
}

func TestMeasureDownloadUsesProbeTimeoutAndRecordsMetrics(t *testing.T) {
	downloadProbe := &fakeDownloadProbe{}
	cfg := config.Default()
	target := config.DownloadProbeConfig{
		Name:            "r2_1mb",
		URL:             "https://example.com/netwatch-1mb.bin",
		Label:           "R2 1MB",
		ExpectedBytes:   1048576,
		TimeoutSeconds:  3,
		IntervalSeconds: 600,
		Enabled:         true,
		DisplayOrder:    10,
	}
	collector := New(cfg, nil, nil, nil, downloadProbe, nil, NewState())

	before := time.Now()
	sample := collector.measureDownload(context.Background(), target)
	remaining := time.Until(downloadProbe.deadline)

	if sample.Type != "download" || sample.URL != target.URL || sample.DisplayName != "R2 1MB" || sample.DisplayOrder != 10 || sample.ExpectedBytes == nil || *sample.ExpectedBytes != target.ExpectedBytes {
		t.Fatalf("sample = %+v, want download metadata", sample)
	}
	if sample.DownloadedBytes == nil || *sample.DownloadedBytes != target.ExpectedBytes || sample.Mbps == nil || *sample.Mbps <= 0 {
		t.Fatalf("sample = %+v, want download metrics", sample)
	}
	if downloadProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want probe timeout around 3s", remaining)
	}
}

func TestMeasureStatusPageUsesMetadataAndTimeout(t *testing.T) {
	statusPageProbe := &fakeStatusPageProbe{}
	cfg := config.Default()
	cfg.HTTPTimeoutSeconds = 3
	target := config.StatusPageConfig{
		Name:                "github_status",
		Label:               "GitHub Status",
		DisplayOrder:        10,
		Type:                "status_page",
		Provider:            "statuspage",
		Group:               "github",
		Category:            "dev",
		URL:                 "https://www.githubstatus.com/api/v2/summary.json",
		ImportantComponents: []string{"API Requests"},
	}
	collector := New(cfg, nil, nil, nil, nil, nil, NewState(), statusPageProbe)

	before := time.Now()
	sample := collector.measureStatusPage(context.Background(), target)
	remaining := time.Until(statusPageProbe.deadline)

	if sample.Type != "status_page" || sample.Name != "github_status" || sample.DisplayName != "GitHub Status" || sample.DisplayOrder != 10 || sample.Provider != "statuspage" || sample.Group != "github" || sample.Category != "dev" {
		t.Fatalf("sample = %+v, want status page metadata", sample)
	}
	if sample.Level != "ok" || sample.Indicator != "none" || sample.Description != "All Systems Operational" || sample.DurationMs == nil || *sample.DurationMs != 123 || sample.OK == nil || !*sample.OK {
		t.Fatalf("sample = %+v, want status page result", sample)
	}
	if len(statusPageProbe.importantComponents) != 1 || statusPageProbe.importantComponents[0] != "API Requests" {
		t.Fatalf("importantComponents = %+v, want configured components", statusPageProbe.importantComponents)
	}
	if statusPageProbe.deadline.Before(before.Add(2*time.Second)) || remaining > 3*time.Second {
		t.Fatalf("deadline remaining = %v, want HTTP timeout around 3s", remaining)
	}
}

func TestCollectRemoteSpeedProbeStoresCompletedSamplesAndDedupes(t *testing.T) {
	measuredAt := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	speedprobeClient := &fakeSpeedprobeClient{
		latest: speedprobe.LatestResponse{
			Observer: speedprobe.Observer{
				Hostname:  "scum",
				Interface: "eth0",
				LinkSpeed: "1000Mb/s",
				Duplex:    "full",
				Operstate: "up",
			},
			Probes: []speedprobe.Probe{
				{
					Name:            "r2_10mb",
					Label:           "R2 10MB",
					Status:          "ok",
					Enabled:         true,
					URL:             "https://example.com/10mb.bin",
					ExpectedBytes:   int64Ptr(10485760),
					DownloadedBytes: int64Ptr(10485760),
					DurationMs:      floatPtr(1200),
					Mbps:            floatPtr(69.9),
					MeasuredAt:      &measuredAt,
					LastRunID:       "run-1",
				},
				{
					Name:       "r2_100mb",
					Status:     "running",
					Running:    true,
					MeasuredAt: &measuredAt,
				},
			},
		},
	}
	storage := &fakeStorage{}
	state := NewState()
	collector := New(config.Default(), nil, nil, nil, nil, storage, state).WithSpeedprobe(speedprobeClient)
	target := config.RemoteSpeedProbeConfig{
		Name:            "scum_speedprobe",
		Label:           "Scum Speedprobe",
		URL:             "http://scum:8090/api/v1/speed/latest",
		IntervalSeconds: 300,
		TimeoutSeconds:  3,
		Enabled:         true,
		DisplayOrder:    30,
	}

	collector.collectRemoteSpeedProbe(context.Background(), target)
	collector.collectRemoteSpeedProbe(context.Background(), target)

	if len(storage.samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1 completed deduped sample", len(storage.samples))
	}
	sample := storage.samples[0]
	if sample.Type != "speedprobe" || sample.Kind != "speedprobe" || sample.Source != "scum_speedprobe" || sample.SourceLabel != "Scum Speedprobe" || sample.Name != "r2_10mb" || sample.DisplayName != "R2 10MB" || sample.DisplayOrder != 30 || sample.RunID != "run-1" {
		t.Fatalf("sample = %+v, want speedprobe metadata", sample)
	}
	if sample.Observer == nil || sample.Observer.Hostname != "scum" || sample.Mbps == nil || *sample.Mbps != 69.9 || sample.ExpectedBytes == nil || *sample.ExpectedBytes != 10485760 || sample.OK == nil || !*sample.OK {
		t.Fatalf("sample = %+v, want speedprobe metrics", sample)
	}
	if got := state.LatestByType("speedprobe"); len(got) != 1 || got[0].Source != "scum_speedprobe" {
		t.Fatalf("LatestByType(speedprobe) = %+v, want stored source sample", got)
	}
	if speedprobeClient.timeout != 3*time.Second {
		t.Fatalf("timeout = %v, want 3s", speedprobeClient.timeout)
	}
}

func TestCollectRemoteSpeedProbeDedupesLoadedLatestState(t *testing.T) {
	measuredAt := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	speedprobeClient := &fakeSpeedprobeClient{
		latest: speedprobe.LatestResponse{
			Probes: []speedprobe.Probe{
				{
					Name:       "r2_10mb",
					Label:      "R2 10MB",
					Status:     "ok",
					Enabled:    true,
					MeasuredAt: &measuredAt,
					LastRunID:  "run-1",
				},
			},
		},
	}
	state := NewState()
	state.Load([]model.Sample{
		{Timestamp: measuredAt, Type: "speedprobe", Kind: "speedprobe", Source: "scum_speedprobe", Name: "r2_10mb", RunID: "run-1"},
	})
	storage := &fakeStorage{}
	collector := New(config.Default(), nil, nil, nil, nil, storage, state).WithSpeedprobe(speedprobeClient)

	collector.collectRemoteSpeedProbe(context.Background(), config.RemoteSpeedProbeConfig{
		Name:            "scum_speedprobe",
		URL:             "http://scum:8090/api/v1/speed/latest",
		IntervalSeconds: 300,
		TimeoutSeconds:  3,
		Enabled:         true,
	})

	if len(storage.samples) != 0 {
		t.Fatalf("samples = %+v, want loaded latest state to dedupe same run", storage.samples)
	}
}

func TestDownloadRetryStateTransitions(t *testing.T) {
	cfg := config.Default()
	collector := New(cfg, nil, nil, nil, nil, nil, NewState())
	target := retryDownloadProbeConfig()
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)

	okSample := downloadSample("r2_1mb", true, 8)
	next := collector.updateDownloadRetry(target, &okSample, now)
	if okSample.RetryState != "normal" || next != now.Add(10*time.Minute) {
		t.Fatalf("ok sample = %+v next=%v, want normal interval", okSample, next)
	}

	alertSample := downloadSample("r2_1mb", true, 3.2)
	next = collector.updateDownloadRetry(target, &alertSample, now)
	if alertSample.RetryState != "degraded" || intValue(alertSample.RetryAttempt) != 0 || intValue(alertSample.RecoverySuccessCount) != 0 || next != now.Add(10*time.Second) {
		t.Fatalf("alert sample = %+v next=%v, want degraded first retry", alertSample, next)
	}

	alertAgain := downloadSample("r2_1mb", true, 3.2)
	next = collector.updateDownloadRetry(target, &alertAgain, now.Add(10*time.Second))
	if alertAgain.RetryState != "degraded" || intValue(alertAgain.RetryAttempt) != 1 || next != now.Add(10*time.Second).Add(30*time.Second) {
		t.Fatalf("alert again = %+v next=%v, want retry attempt 1", alertAgain, next)
	}

	recovering := downloadSample("r2_1mb", true, 8)
	next = collector.updateDownloadRetry(target, &recovering, now.Add(40*time.Second))
	if recovering.RetryState != "recovering" || intValue(recovering.RetryAttempt) != 1 || intValue(recovering.RecoverySuccessCount) != 1 || next != now.Add(40*time.Second).Add(30*time.Second) {
		t.Fatalf("recovering = %+v next=%v, want recovering retry", recovering, next)
	}

	recovered := downloadSample("r2_1mb", true, 8)
	next = collector.updateDownloadRetry(target, &recovered, now.Add(70*time.Second))
	if recovered.RetryState != "normal" || intValue(recovered.RetryAttempt) != 0 || intValue(recovered.RecoverySuccessCount) != 0 || next != now.Add(70*time.Second).Add(10*time.Minute) {
		t.Fatalf("recovered = %+v next=%v, want normal recovery", recovered, next)
	}
}

func TestDownloadRetryRecoveringAlertReturnsToDegraded(t *testing.T) {
	collector := New(config.Default(), nil, nil, nil, nil, nil, NewState())
	target := retryDownloadProbeConfig()
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)

	alertSample := downloadSample("r2_1mb", true, 3.2)
	collector.updateDownloadRetry(target, &alertSample, now)
	okSample := downloadSample("r2_1mb", true, 8)
	collector.updateDownloadRetry(target, &okSample, now.Add(10*time.Second))
	alertAgain := downloadSample("r2_1mb", true, 3.2)
	next := collector.updateDownloadRetry(target, &alertAgain, now.Add(20*time.Second))

	if alertAgain.RetryState != "degraded" || intValue(alertAgain.RetryAttempt) != 1 || intValue(alertAgain.RecoverySuccessCount) != 0 || next != now.Add(20*time.Second).Add(30*time.Second) {
		t.Fatalf("alert again = %+v next=%v, want degraded after recovering alert", alertAgain, next)
	}
}

func TestDownloadRetryUsesLastIntervalAfterLastAttempt(t *testing.T) {
	collector := New(config.Default(), nil, nil, nil, nil, nil, NewState())
	target := retryDownloadProbeConfig()
	runtime := collector.downloadRetryState(target.Name)
	runtime.State = "degraded"
	runtime.Attempt = 10
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)

	alertSample := downloadSample("r2_1mb", true, 3.2)
	next := collector.updateDownloadRetry(target, &alertSample, now)

	if intValue(alertSample.RetryAttempt) != 11 || next != now.Add(10*time.Minute) {
		t.Fatalf("alert sample = %+v next=%v, want last retry interval", alertSample, next)
	}
}

func TestDownloadRetryDisabledUsesNormalIntervalWithoutMetadata(t *testing.T) {
	storage := &fakeStorage{}
	state := NewState()
	cfg := config.Default()
	target := config.DownloadProbeConfig{
		Name:            "r2_1mb",
		URL:             "https://example.com/netwatch-1mb.bin",
		ExpectedBytes:   1048576,
		TimeoutSeconds:  3,
		IntervalSeconds: 600,
		Enabled:         true,
	}
	downloadProbe := &fakeDownloadProbe{}
	collector := New(cfg, nil, nil, nil, downloadProbe, storage, state)
	now := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)

	next := collector.collectDownload(context.Background(), target, now)

	if next != now.Add(10*time.Minute) {
		t.Fatalf("next = %v, want normal interval", next)
	}
	if len(storage.samples) != 1 || storage.samples[0].RetryState != "" || storage.samples[0].NextCheckAt != nil {
		t.Fatalf("samples = %+v, want no retry metadata when disabled", storage.samples)
	}
}

func retryDownloadProbeConfig() config.DownloadProbeConfig {
	return config.DownloadProbeConfig{
		Name:            "r2_1mb",
		URL:             "https://example.com/netwatch-1mb.bin",
		ExpectedBytes:   1048576,
		TimeoutSeconds:  3,
		IntervalSeconds: 600,
		Enabled:         true,
		RetryOnAlert: config.RetryOnAlertConfig{
			Enabled:              true,
			IntervalsSeconds:     []int{10, 30, 600},
			RecoverySuccessCount: 2,
		},
	}
}

func downloadSample(name string, ok bool, mbps float64) model.Sample {
	return model.Sample{
		Timestamp: time.Now(),
		Type:      "download",
		Name:      name,
		OK:        boolPtr(ok),
		Mbps:      &mbps,
	}
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func floatPtr(value float64) *float64 {
	return &value
}
