package store

import (
	"testing"
	"time"
)

func TestFormatTimeFixedWidthUTC(t *testing.T) {
	loc := time.FixedZone("UTC-6", -6*3600)
	tZeroNano := time.Date(2026, 5, 1, 12, 0, 0, 0, loc)
	tWithNano := time.Date(2026, 5, 1, 12, 0, 0, 123456789, loc)

	sZero := formatTime(tZeroNano)
	sNano := formatTime(tWithNano)

	if sZero != "2026-05-01T18:00:00.000000000Z" {
		t.Fatalf("sZero = %q, want 2026-05-01T18:00:00.000000000Z", sZero)
	}
	if sNano != "2026-05-01T18:00:00.123456789Z" {
		t.Fatalf("sNano = %q, want 2026-05-01T18:00:00.123456789Z", sNano)
	}
	// En SQLite la comparación TEXT exige que ".000000000Z" < ".123456789Z"
	// (con RFC3339Nano, "2026-05-01T18:00:00Z" > "2026-05-01T18:00:00.123Z" porque 'Z' > '.').
	if !(sZero < sNano) {
		t.Fatalf("orden lexicográfico roto: %q no es < %q", sZero, sNano)
	}
	if got, err := parseTime(sZero); err != nil || !got.Equal(tZeroNano) {
		t.Fatalf("parseTime(%q) = (%v, %v), want %v", sZero, got, err, tZeroNano)
	}
}
