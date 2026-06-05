package api

import "testing"

func TestParseRange(t *testing.T) {
	tests := map[string]int{
		"1h":  1,
		"6h":  6,
		"24h": 24,
		"7d":  24 * 7,
	}

	for value, wantHours := range tests {
		got, err := parseRange(value)
		if err != nil {
			t.Fatalf("parseRange(%q) error = %v", value, err)
		}
		if int(got.Hours()) != wantHours {
			t.Fatalf("parseRange(%q) = %v hours, want %d", value, got.Hours(), wantHours)
		}
	}
}

func TestParseRangeUnsupported(t *testing.T) {
	if _, err := parseRange("30m"); err == nil {
		t.Fatal("parseRange() error = nil, want error")
	}
}
