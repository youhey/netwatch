package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPGetOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != UserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, UserAgent)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	result, err := HTTP{}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.HTTPStatus == nil || *result.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus = %v, want 200", result.HTTPStatus)
	}
	if result.TotalMs < 0 {
		t.Fatalf("TotalMs = %v, want >= 0", result.TotalMs)
	}
	if result.TTFBMs == nil || *result.TTFBMs < 0 {
		t.Fatalf("TTFBMs = %v, want >= 0", result.TTFBMs)
	}
	if result.ContentLengthRead != 2 {
		t.Fatalf("ContentLengthRead = %d, want 2", result.ContentLengthRead)
	}
	if result.BodyTruncated {
		t.Fatal("BodyTruncated = true, want false")
	}
}

func TestNewHTTPConfiguresKeepAlive(t *testing.T) {
	probe := NewHTTP(true, 262144)
	transport, ok := probe.Client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", probe.Client.Transport)
	}
	if !transport.DisableKeepAlives {
		t.Fatal("DisableKeepAlives = false, want true")
	}

	probe = NewHTTP(false, 262144)
	transport, ok = probe.Client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", probe.Client.Transport)
	}
	if transport.DisableKeepAlives {
		t.Fatal("DisableKeepAlives = true, want false")
	}
}

func TestHTTPGetNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := HTTP{}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.HTTPStatus == nil || *result.HTTPStatus != http.StatusNoContent {
		t.Fatalf("HTTPStatus = %v, want 204", result.HTTPStatus)
	}
}

func TestHTTPGetForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	result, err := HTTP{}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if result.HTTPStatus == nil || *result.HTTPStatus != http.StatusForbidden {
		t.Fatalf("HTTPStatus = %v, want 403", result.HTTPStatus)
	}
}

func TestHTTPGetServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result, err := HTTP{}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if result.HTTPStatus == nil || *result.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("HTTPStatus = %v, want 500", result.HTTPStatus)
	}
}

func TestHTTPGetBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 20)))
	}))
	defer server.Close()

	result, err := HTTP{MaxBodyBytes: 5}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.ContentLengthRead != 5 {
		t.Fatalf("ContentLengthRead = %d, want 5", result.ContentLengthRead)
	}
	if !result.BodyTruncated {
		t.Fatal("BodyTruncated = false, want true")
	}
}

func TestHTTPGetTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := HTTP{Client: server.Client()}.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.TLSMs == nil || *result.TLSMs < 0 {
		t.Fatalf("TLSMs = %v, want >= 0", result.TLSMs)
	}
}

func TestHTTPGetTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	result, err := HTTP{}.Get(ctx, server.URL)
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if result.TotalMs < 0 {
		t.Fatalf("TotalMs = %v, want >= 0", result.TotalMs)
	}
}
