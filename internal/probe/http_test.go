package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
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
