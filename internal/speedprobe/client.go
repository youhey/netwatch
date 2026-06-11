package speedprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const UserAgent = "netwatch-speedprobe-pull/dev"

type Client struct {
	HTTPClient *http.Client
}

func NewClient() Client {
	return Client{}
}

func (c Client) Latest(ctx context.Context, url string, timeout time.Duration) (LatestResponse, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestResponse{}, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return LatestResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LatestResponse{}, fmt.Errorf("speedprobe latest returned HTTP %d", resp.StatusCode)
	}

	var latest LatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return LatestResponse{}, err
	}
	return latest, nil
}
