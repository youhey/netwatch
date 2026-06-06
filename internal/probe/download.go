package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DownloadUserAgent = "netwatch-download-probe/dev"

type DownloadResult struct {
	OK              bool
	HTTPStatus      *int
	DownloadedBytes int64
	DurationMs      float64
	BytesPerSec     float64
	Mbps            float64
}

type Download struct {
	Client *http.Client
}

func NewDownload() Download {
	return Download{
		Client: &http.Client{},
	}
}

func (p Download) Get(ctx context.Context, url string, expectedBytes int64) (DownloadResult, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DownloadResult{}, err
	}
	req.Header.Set("User-Agent", DownloadUserAgent)

	start := time.Now()
	resp, err := client.Do(req)
	result := DownloadResult{DurationMs: positiveDurationMs(start, time.Now())}
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	result.HTTPStatus = &status
	if status != http.StatusOK {
		return result, fmt.Errorf("http status %d", status)
	}

	downloadedBytes, err := io.Copy(io.Discard, resp.Body)
	result.DownloadedBytes = downloadedBytes
	result.DurationMs = positiveDurationMs(start, time.Now())
	setDownloadRates(&result)
	if err != nil {
		return result, err
	}
	if expectedBytes > 0 && downloadedBytes != expectedBytes {
		return result, fmt.Errorf("downloaded bytes %d did not match expected bytes %d", downloadedBytes, expectedBytes)
	}
	if expectedBytes <= 0 && downloadedBytes <= 0 {
		return result, fmt.Errorf("downloaded bytes must be greater than 0")
	}

	result.OK = result.DurationMs > 0
	return result, nil
}

func setDownloadRates(result *DownloadResult) {
	durationSec := result.DurationMs / 1000
	if durationSec <= 0 {
		return
	}
	result.BytesPerSec = float64(result.DownloadedBytes) / durationSec
	result.Mbps = float64(result.DownloadedBytes) * 8 / durationSec / 1_000_000
}

func positiveDurationMs(start, end time.Time) float64 {
	ms := durationMs(start, end)
	if ms <= 0 {
		return 0.001
	}
	return ms
}
