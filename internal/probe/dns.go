package probe

import (
	"context"
	"net"
	"sort"
	"time"
)

type DNSResult struct {
	OK          bool
	DurationMs  float64
	ResolvedIPs []string
}

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type DNS struct {
	Resolver Resolver
}

func (p DNS) Lookup(ctx context.Context, hostname string) (DNSResult, error) {
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	start := time.Now()
	addrs, err := resolver.LookupIPAddr(ctx, hostname)
	duration := durationMs(start, time.Now())

	result := DNSResult{
		OK:         err == nil,
		DurationMs: duration,
	}
	if err != nil {
		return result, err
	}

	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP.String())
	}
	sort.Strings(ips)
	result.ResolvedIPs = ips

	return result, nil
}
