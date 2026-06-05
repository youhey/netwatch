package probe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

type PingResult struct {
	Sent        int
	Received    int
	LossPercent float64
	RTTMinMs    *float64
	RTTAvgMs    *float64
	RTTMaxMs    *float64
}

type Fping struct{}

func (Fping) Ping(ctx context.Context, target string, count int) (PingResult, error) {
	if count <= 0 {
		return PingResult{}, errors.New("ping count must be greater than 0")
	}

	cmd := exec.CommandContext(ctx, "fping", "-C", strconv.Itoa(count), "-q", target)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return PingResult{}, ctx.Err()
	}

	result, parseErr := ParseFpingOutput(string(out), count)
	if parseErr != nil {
		if err != nil {
			return PingResult{}, fmt.Errorf("%w: %v", err, parseErr)
		}
		return PingResult{}, parseErr
	}
	if err != nil {
		return result, err
	}

	return result, nil
}

func ParseFpingOutput(output string, fallbackSent int) (PingResult, error) {
	line := ""
	for _, candidate := range strings.Split(output, "\n") {
		if strings.Contains(candidate, ":") {
			line = strings.TrimSpace(candidate)
		}
	}
	if line == "" {
		return PingResult{}, errors.New("fping output does not contain a result line")
	}

	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return PingResult{}, fmt.Errorf("invalid fping result line: %q", line)
	}

	values := strings.Fields(parts[1])
	sent := len(values)
	if sent == 0 {
		sent = fallbackSent
	}
	if sent <= 0 {
		return PingResult{}, errors.New("fping result does not contain samples")
	}

	var received int
	var sum float64
	min := math.Inf(1)
	max := math.Inf(-1)
	for _, value := range values {
		if value == "-" {
			continue
		}

		rtt, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return PingResult{}, fmt.Errorf("invalid fping RTT value %q: %w", value, err)
		}

		received++
		sum += rtt
		if rtt < min {
			min = rtt
		}
		if rtt > max {
			max = rtt
		}
	}

	result := PingResult{
		Sent:        sent,
		Received:    received,
		LossPercent: float64(sent-received) / float64(sent) * 100,
	}
	if received > 0 {
		avg := sum / float64(received)
		result.RTTMinMs = &min
		result.RTTAvgMs = &avg
		result.RTTMaxMs = &max
	}

	return result, nil
}
