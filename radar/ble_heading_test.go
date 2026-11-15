package radar

import (
	"math"
	"testing"
)

func TestLooksLikeRandomAddress(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"C0:11:22:33:44:55", true},  // static random (11)
		{"42:11:22:33:44:55", true},  // resolvable private (01)
		{"00:11:22:33:44:55", false}, // 00
		{"80:11:22:33:44:55", false}, // 10
		{"not-hex:11:22", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := LooksLikeRandomAddress(tc.addr); got != tc.want {
			t.Errorf("LooksLikeRandomAddress(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestAggregateBleDevicesAndStates(t *testing.T) {
	list := AggregateBleDevices(nil, BleDevice{Address: "02:00:00:00:00:01", Rssi: -70, LastSeenMillis: 100})
	list = AggregateBleDevices(list, BleDevice{Address: "02:00:00:00:00:02", Rssi: -60, LastSeenMillis: 200})
	list = AggregateBleDevices(list, BleDevice{Address: "02:00:00:00:00:01", Rssi: -55, LastSeenMillis: 300})
	if len(list) != 2 || list[0].Address != "02:00:00:00:00:01" || list[0].Rssi != -55 {
		t.Fatalf("unexpected aggregated list: %+v", list)
	}

	if got := BleScanState(false, true); got != SignalStateNoPermission {
		t.Fatalf("BleScanState(false, true) = %v", got)
	}
	if got := BleScanState(true, false); got != SignalStateRadioOff {
		t.Fatalf("BleScanState(true, false) = %v", got)
	}
	if got := BleScanState(true, true); got != SignalStateOK {
		t.Fatalf("BleScanState(true, true) = %v", got)
	}
}

func TestWifiSignalHelpers(t *testing.T) {
	if !BssidEquals("aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF") {
		t.Fatal("expected case-insensitive BSSID match")
	}
	if BssidEquals("", "AA:BB:CC:DD:EE:FF") {
		t.Fatal("empty BSSID must not match")
	}
	if got := WifiConnectedState(true, true, false); got != SignalStateNotFound {
		t.Fatalf("WifiConnectedState(not connected) = %v", got)
	}
	if got := WifiScanState(true, true, false, true, 5, 3); got != SignalStateSlowThrottled {
		t.Fatalf("WifiScanState(throttled) = %v", got)
	}
	if got := WifiScanState(true, true, false, false, 3, 3); got != SignalStateNotFound {
		t.Fatalf("WifiScanState(3 misses) = %v", got)
	}
	if got := WifiScanState(true, true, false, false, 2, 3); got != SignalStateOK {
		t.Fatalf("WifiScanState(2 misses) = %v", got)
	}
}

func TestHeadingMath(t *testing.T) {
	if got := NormalizeDegrees(-10); got != 350 {
		t.Fatalf("NormalizeDegrees(-10) = %v", got)
	}
	if got := NormalizeDegrees(360); got != 0 {
		t.Fatalf("NormalizeDegrees(360) = %v", got)
	}

	// Identity rotation matrix: top edge points north (0 deg), pitch 0.
	id := [9]float64{
		1, 0, 0,
		0, 1, 0,
		0, 0, 1,
	}
	if got := FlatAzimuthDeg(id); math.Abs(got) > 1e-6 {
		t.Fatalf("FlatAzimuthDeg(id) = %v", got)
	}
	if got := PitchDeg(id); math.Abs(got) > 1e-6 {
		t.Fatalf("PitchDeg(id) = %v", got)
	}
	if got := PostureAwareAzimuthDeg(id, 45); math.Abs(got) > 1e-6 {
		t.Fatalf("PostureAwareAzimuthDeg(id) = %v", got)
	}

	// Phone tilted upright (90 deg pitch around X): R[7] = -1, rear camera (-Z) points North -> R[5] = -1.
	upright := [9]float64{
		1, 0, 0,
		0, 0, -1,
		0, -1, 0,
	}
	if got := VerticalAzimuthDeg(upright); math.Abs(got) > 1e-6 {
		t.Fatalf("VerticalAzimuthDeg(upright) = %v, want 0", got)
	}
	if got := PostureAwareAzimuthDeg(upright, 45); math.Abs(got) > 1e-6 {
		t.Fatalf("PostureAwareAzimuthDeg(upright) = %v, want 0", got)
	}

	// Circular smoothing across 359 -> 1 must stay near 0, not 180.
	smoothed := SmoothAzimuthDeg(359, true, 1, 0.5)
	if AngleErrorDeg(smoothed, 0) > 0.5 {
		t.Fatalf("SmoothAzimuthDeg(359, 1) = %v, want ~0", smoothed)
	}

	if x, y := ScreenRemapAxes(1); x != AxisY || y != AxisMinusX {
		t.Fatalf("ScreenRemapAxes(1) = (%d, %d)", x, y)
	}
	if x, y := ScreenRemapAxes(99); x != AxisX || y != AxisY {
		t.Fatalf("ScreenRemapAxes(99) = (%d, %d)", x, y)
	}
}
