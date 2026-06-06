package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDownloadSuccess(t *testing.T) {
	body := []byte("0123456789")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != DownloadUserAgent {
			t.Fatalf("User-Agent = %q, want %q", r.UserAgent(), DownloadUserAgent)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	result, err := NewDownload().Get(context.Background(), server.URL, int64(len(body)))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK || result.DownloadedBytes != int64(len(body)) || result.DurationMs <= 0 {
		t.Fatalf("result = %+v, want successful download", result)
	}
}

func TestDownloadExpectedBytesMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	result, err := NewDownload().Get(context.Background(), server.URL, 10)
	if err == nil {
		t.Fatal("Get() error = nil, want mismatch error")
	}
	if result.OK || result.DownloadedBytes != 5 {
		t.Fatalf("result = %+v, want failed mismatch with downloaded bytes", result)
	}
}

func TestDownloadTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, err := NewDownload().Get(ctx, server.URL, 4)
	if err == nil {
		t.Fatal("Get() error = nil, want timeout error")
	}
	if result.OK {
		t.Fatalf("result = %+v, want failed timeout", result)
	}
}

func TestDownloadHTTPStatusFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	result, err := NewDownload().Get(context.Background(), server.URL, 0)
	if err == nil {
		t.Fatal("Get() error = nil, want status error")
	}
	if result.OK || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusNotFound {
		t.Fatalf("result = %+v, want 404 failure", result)
	}
}

func TestSetDownloadRates(t *testing.T) {
	result := DownloadResult{
		DownloadedBytes: 1_000_000,
		DurationMs:      2000,
	}
	setDownloadRates(&result)

	if result.BytesPerSec != 500_000 || result.Mbps != 4 {
		t.Fatalf("result = %+v, want 500000 Bps and 4 Mbps", result)
	}
}
