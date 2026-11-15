package mobile

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/core"
	"github.com/baxner/vigia/radar"
)

func TestScanConfigAndInterfaceWrappers(t *testing.T) {
	cfg := NewScanConfig()
	if cfg == nil || !cfg.WithPorts {
		t.Fatalf("unexpected NewScanConfig: %+v", cfg)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	sc := NewScanner()
	portsJSON, err := sc.ScanPortsOnInterface("127.0.0.1", fmt.Sprintf("%d", port), "nonexistent-vigia-iface-0")
	if err != nil {
		t.Fatalf("ScanPortsOnInterface: %v", err)
	}
	var open []int
	if err := json.Unmarshal([]byte(portsJSON), &open); err != nil {
		t.Fatalf("unmarshal ports: %v (%s)", err, portsJSON)
	}
	if len(open) != 1 || open[0] != port {
		t.Fatalf("open = %v, want [%d]", open, port)
	}

	// ScanOnInterface with nonexistent interface must fall back cleanly.
	scanJSON, err := sc.ScanOnInterface("127.0.0.1/32", "", false, "nonexistent-vigia-iface-0", nil)
	if err != nil {
		t.Fatalf("ScanOnInterface: %v", err)
	}
	var sr core.ScanResult
	if err := json.Unmarshal([]byte(scanJSON), &sr); err != nil {
		t.Fatalf("unmarshal ScanResult: %v", err)
	}

	// ScanWithConfig handles nil config gracefully if CIDR is empty (may fail only if no active non-loopback LAN).
	cfg2 := &ScanConfig{
		CIDR:              "127.0.0.1/32",
		WithPorts:         false,
		BindIface:         "nonexistent-vigia-iface-0",
		SkipNeighborTable: true,
	}
	if _, err := sc.ScanWithConfig(cfg2, nil); err != nil {
		t.Fatalf("ScanWithConfig: %v", err)
	}

	if _, err := RouterInfoOnInterface("not-an-ip", "en0"); err == nil {
		t.Fatal("expected error for invalid gateway")
	}
	rJSON, err := RouterInfoOnInterface("127.0.0.1", "nonexistent-vigia-iface-0")
	if err != nil {
		t.Fatalf("RouterInfoOnInterface error: %v", err)
	}
	if rJSON != "null" {
		t.Fatalf("RouterInfoOnInterface = %s, want null", rJSON)
	}
}

func TestMobileRadarWrappers(t *testing.T) {
	if cnt := BluetoothCompanyCount(); cnt < 4000 {
		t.Fatalf("BluetoothCompanyCount() = %d", cnt)
	}
	if got := BluetoothCompanyFullName(76); got != "Apple, Inc." {
		t.Fatalf("full name = %q", got)
	}
	if got := BluetoothCompanyBrand(76); got != "Apple" {
		t.Fatalf("brand = %q", got)
	}
	if got := BluetoothShortName("Samsung Electronics Co. Ltd."); got != "Samsung Electronics" {
		t.Fatalf("short = %q", got)
	}

	f := NewBleRssiFilter()
	if f.HasValue() {
		t.Fatal("new filter should not have value")
	}
	v1 := f.Update(-60)
	if !f.HasValue() || v1 != -60 || f.Value() != -60 {
		t.Fatalf("after first update: has=%v v1=%v val=%v", f.HasValue(), v1, f.Value())
	}
	_ = f.Update(-64)
	f.Reset()
	if f.HasValue() {
		t.Fatal("expected Reset to clear value")
	}
	_ = NewWifiRssiFilter().Update(-55)
	_ = NewRssiFilter(0.5, 4.0).Update(-58)

	dr := EstimateDistance(-50, WifiReferenceAt1m(2437))
	if dr == nil || !(dr.MinMeters <= dr.TypicalMeters && dr.TypicalMeters <= dr.MaxMeters) {
		t.Fatalf("invalid distance range: %+v", dr)
	}
	if BleReferenceAt1m(-5, true) != -46 || BleReferenceAt1m(0, false) != -62 {
		t.Fatal("unexpected BLE 1m reference")
	}
	if ProximityFraction(-95) != 0 || ProximityFraction(-35) != 1 {
		t.Fatal("unexpected proximity bounds")
	}
	if GeigerIntervalMillis(-100) <= GeigerIntervalMillis(-40) {
		t.Fatal("expected slower click interval for weaker RSSI")
	}

	td := NewTrendDetector(3000, 2.0)
	td.Update(0, -70)
	if got := td.Update(3000, -65); got != string(radar.TrendCloser) {
		t.Fatalf("trend = %q, want CLOSER", got)
	}
	td.Reset()

	be := NewBearingEstimator()
	var samples []radar.AzimuthRssi
	for sector := 0; sector < 8; sector++ {
		for i := 0; i < 3; i++ {
			az := float64(sector)*45 + float64(i)*12 + 5
			sig := -65 + 5*math.Cos((az-90)*math.Pi/180)
			be.AddSample(az, sig)
			samples = append(samples, radar.AzimuthRssi{AzimuthDeg: az, Rssi: sig})
		}
	}
	if be.SampleCount() != 24 || be.SectorCount(BearingSectorOf(90)) != 3 {
		t.Fatalf("counts mismatch: total=%d sec2=%d", be.SampleCount(), be.SectorCount(2))
	}
	if !strings.Contains(be.SectorCountsJSON(), "[3,3,3,3,3,3,3,3]") {
		t.Fatalf("SectorCountsJSON = %s", be.SectorCountsJSON())
	}
	est := be.Estimate(2)
	if !est.Available || math.Abs(ShortestAngleDiff(90, est.BearingDeg)) > 15 {
		t.Fatalf("unexpected bearing estimate: %+v", est)
	}
	rawSamples, _ := json.Marshal(samples)
	estJSON, err := EstimateBearingJSON(string(rawSamples), 2)
	if err != nil || estJSON == "null" {
		t.Fatalf("EstimateBearingJSON = (%s, %v)", estJSON, err)
	}
	be.Reset()
	if be.SampleCount() != 0 {
		t.Fatal("BearingEstimator Reset failed")
	}

	if !LooksLikeRandomBleAddress("C0:11:22:33:44:55") || LooksLikeRandomBleAddress("00:11:22:33:44:55") {
		t.Fatal("unexpected LooksLikeRandomBleAddress result")
	}
	if NormalizeDegrees(-15) != 345 {
		t.Fatal("NormalizeDegrees failed")
	}
	if math.Abs(ShortestAngleDiff(350, 10)-20) > 1e-6 {
		t.Fatal("ShortestAngleDiff wrap failed")
	}
	if got := SmoothAzimuthDeg(359, true, 1, 0.5); math.Abs(ShortestAngleDiff(0, got)) > 1 {
		t.Fatalf("SmoothAzimuthDeg = %v", got)
	}

	sib, err := MacSiblingName("02:00:00:11:22:02", `[{"mac":"02:00:00:11:22:01","name":"Lamp"}]`, 3)
	if err != nil || sib != "Lamp" {
		t.Fatalf("MacSiblingName = (%q, %v)", sib, err)
	}
}

func TestMobileHistoryWrappers(t *testing.T) {
	if got := NetworkKey("192.0.2.1", "192.0.2.0/24"); got != "192.0.2.1|192.0.2.0/24" {
		t.Fatalf("NetworkKey = %q", got)
	}

	devJSON := `{"ip":"192.0.2.10","mac":"02:00:00:aa:bb:cc","hostname":"NAS-Box","extra":{"mdns:name":"MyNAS"}}`
	name, err := StrongNameOf(devJSON)
	if err != nil || name != "mynas" {
		t.Fatalf("StrongNameOf = (%q, %v)", name, err)
	}

	mac := "02:00:00:aa:bb:cc"
	known := []history.KnownDevice{{
		ID:      1,
		Network: history.NewNetworkKey("192.0.2.1", "192.0.2.0/24"),
		MAC:     &mac,
		LastIP:  "192.0.2.10",
		Online:  true,
		Last:    core.Device{IP: "192.0.2.10", MAC: mac},
	}}
	scanned := []core.Device{{IP: "192.0.2.20", MAC: mac, Hostname: "NAS-Box"}}
	kb, _ := json.Marshal(known)
	sb, _ := json.Marshal(scanned)

	matchesJSON, err := ReconcileDevices(string(kb), string(sb))
	if err != nil {
		t.Fatalf("ReconcileDevices: %v", err)
	}
	var matches []history.Match
	if err := json.Unmarshal([]byte(matchesJSON), &matches); err != nil {
		t.Fatalf("unmarshal matches: %v", err)
	}
	if len(matches) != 1 || matches[0].Known == nil || matches[0].Known.ID != 1 {
		t.Fatalf("unexpected matches: %+v", matches)
	}

	prevJSON, _ := json.Marshal(known[0])
	scanOneJSON, _ := json.Marshal(scanned[0])
	seenOutJSON, err := HistorySeen(string(prevJSON), string(scanOneJSON), 1700000000000)
	if err != nil {
		t.Fatalf("HistorySeen: %v", err)
	}
	var seenOut history.Outcome
	if err := json.Unmarshal([]byte(seenOutJSON), &seenOut); err != nil {
		t.Fatalf("unmarshal seenOut: %v", err)
	}
	if seenOut.Device.LastIP != "192.0.2.20" || len(seenOut.Events) != 1 || seenOut.Events[0].Type != history.IPChanged {
		t.Fatalf("unexpected seen outcome: %+v", seenOut)
	}

	missOutJSON, err := HistoryMissing(string(prevJSON), 1, 1700000900000)
	if err != nil {
		t.Fatalf("HistoryMissing: %v", err)
	}
	var missOut history.Outcome
	if err := json.Unmarshal([]byte(missOutJSON), &missOut); err != nil {
		t.Fatalf("unmarshal missOut: %v", err)
	}
	if missOut.Device.Online || missOut.OfflineMisses != 2 {
		t.Fatalf("unexpected miss outcome: %+v", missOut)
	}
}
