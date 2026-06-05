package probe

import (
	"math"
	"testing"
)

func TestParseFpingOutput(t *testing.T) {
	result, err := ParseFpingOutput("1.1.1.1 : 8.10 10.40 - 16.90\n", 4)
	if err != nil {
		t.Fatalf("ParseFpingOutput() error = %v", err)
	}

	if result.Sent != 4 {
		t.Fatalf("Sent = %d, want 4", result.Sent)
	}
	if result.Received != 3 {
		t.Fatalf("Received = %d, want 3", result.Received)
	}
	if result.LossPercent != 25 {
		t.Fatalf("LossPercent = %v, want 25", result.LossPercent)
	}
	if result.RTTMinMs == nil || *result.RTTMinMs != 8.10 {
		t.Fatalf("RTTMinMs = %v, want 8.10", result.RTTMinMs)
	}
	if result.RTTAvgMs == nil || math.Abs(*result.RTTAvgMs-11.8) > 0.000001 {
		t.Fatalf("RTTAvgMs = %v, want 11.8", result.RTTAvgMs)
	}
	if result.RTTMaxMs == nil || *result.RTTMaxMs != 16.90 {
		t.Fatalf("RTTMaxMs = %v, want 16.90", result.RTTMaxMs)
	}
}

func TestParseFpingOutputAllLost(t *testing.T) {
	result, err := ParseFpingOutput("8.8.8.8 : - - -\n", 3)
	if err != nil {
		t.Fatalf("ParseFpingOutput() error = %v", err)
	}

	if result.Sent != 3 || result.Received != 0 || result.LossPercent != 100 {
		t.Fatalf("result = %+v, want 3 sent, 0 received, 100%% loss", result)
	}
	if result.RTTMinMs != nil || result.RTTAvgMs != nil || result.RTTMaxMs != nil {
		t.Fatalf("RTT stats = %v/%v/%v, want nil", result.RTTMinMs, result.RTTAvgMs, result.RTTMaxMs)
	}
}
