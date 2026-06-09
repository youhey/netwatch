package probe

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"
)

const UserAgent = "netwatch/0.2"

type HTTPResult struct {
	OK                bool
	HTTPStatus        *int
	DNSMs             *float64
	ConnectMs         *float64
	TLSMs             *float64
	TTFBMs            *float64
	TotalMs           float64
	RemoteAddr        string
	ContentLength     *int64
	ContentLengthRead int64
	BodyTruncated     bool
}

type HTTP struct {
	Client           *http.Client
	DisableKeepAlive bool
	MaxBodyBytes     int64
}

func NewHTTP(disableKeepAlive bool, maxBodyBytes int64) HTTP {
	return HTTP{
		Client: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: disableKeepAlive,
			},
		},
		DisableKeepAlive: disableKeepAlive,
		MaxBodyBytes:     maxBodyBytes,
	}
}

func (p HTTP) Get(ctx context.Context, url string, expectedStatuses []int) (HTTPResult, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: p.DisableKeepAlive,
			},
		}
	}
	maxBodyBytes := p.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 262144
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HTTPResult{}, err
	}
	req.Header.Set("User-Agent", UserAgent)

	var dnsStart, connectStart, tlsStart, requestStart time.Time
	var result HTTPResult

	start := time.Now()
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !dnsStart.IsZero() {
				result.DNSMs = floatPtr(durationMs(dnsStart, time.Now()))
			}
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, addr string, _ error) {
			if !connectStart.IsZero() {
				result.ConnectMs = floatPtr(durationMs(connectStart, time.Now()))
			}
			result.RemoteAddr = addr
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			if !tlsStart.IsZero() {
				result.TLSMs = floatPtr(durationMs(tlsStart, time.Now()))
			}
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			requestStart = time.Now()
		},
		GotFirstResponseByte: func() {
			if !requestStart.IsZero() {
				result.TTFBMs = floatPtr(durationMs(requestStart, time.Now()))
			} else {
				result.TTFBMs = floatPtr(durationMs(start, time.Now()))
			}
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	result.TotalMs = durationMs(start, time.Now())
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	result.HTTPStatus = &status
	result.OK = httpStatusOK(status, expectedStatuses)
	if resp.ContentLength >= 0 {
		result.ContentLength = &resp.ContentLength
	}

	readBytes, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes+1))
	if readBytes > maxBodyBytes {
		result.ContentLengthRead = maxBodyBytes
		result.BodyTruncated = true
	} else {
		result.ContentLengthRead = readBytes
	}
	result.TotalMs = durationMs(start, time.Now())
	if readErr != nil {
		result.OK = false
		return result, readErr
	}

	if !result.OK {
		return result, nil
	}

	return result, nil
}

func httpStatusOK(status int, expectedStatuses []int) bool {
	if len(expectedStatuses) == 0 {
		return status >= 200 && status < 400
	}
	for _, expectedStatus := range expectedStatuses {
		if status == expectedStatus {
			return true
		}
	}
	return false
}

func durationMs(start, end time.Time) float64 {
	return float64(end.Sub(start).Microseconds()) / 1000
}

func floatPtr(value float64) *float64 {
	return &value
}
