package probe

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
)

type fakeResolver struct {
	addrs []net.IPAddr
	err   error
}

func (r fakeResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addrs, r.err
}

func TestDNSLookupOK(t *testing.T) {
	probe := DNS{Resolver: fakeResolver{
		addrs: []net.IPAddr{
			{IP: net.ParseIP("142.250.207.100")},
			{IP: net.ParseIP("142.250.207.99")},
		},
	}}

	result, err := probe.Lookup(context.Background(), "www.google.com")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.DurationMs < 0 {
		t.Fatalf("DurationMs = %v, want >= 0", result.DurationMs)
	}
	want := []string{"142.250.207.100", "142.250.207.99"}
	if !reflect.DeepEqual(result.ResolvedIPs, want) {
		t.Fatalf("ResolvedIPs = %v, want %v", result.ResolvedIPs, want)
	}
}

func TestDNSLookupFailure(t *testing.T) {
	probe := DNS{Resolver: fakeResolver{err: errors.New("lookup failed")}}

	result, err := probe.Lookup(context.Background(), "bad.example")
	if err == nil {
		t.Fatal("Lookup() error = nil, want error")
	}
	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if result.DurationMs < 0 {
		t.Fatalf("DurationMs = %v, want >= 0", result.DurationMs)
	}
}
