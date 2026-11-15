package radar

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestRssiFilterConvergenceAndTracking(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	f := NewBleRssiFilter()
	if f.HasValue() || f.Value() != 0 {
		t.Fatalf("expected uninitialized filter")
	}
	for i := 0; i < 200; i++ {
		f.Update(-60.0 + rng.NormFloat64()*4.0)
	}
	v, ok := f.ValueOK()
	if !ok || math.Abs(v-(-60.0)) > 1.5 {
		t.Fatalf("converged to (%v, %v), want ~-60 +/- 1.5", v, ok)
	}
	f.Reset()
	if f.HasValue() {
		t.Fatal("expected cleared filter after Reset")
	}

	wf := NewWifiRssiFilter()
	for i := 0; i < 10; i++ {
		wf.Update(-75.0)
	}
	for i := 0; i < 8; i++ {
		wf.Update(-55.0)
	}
	if wf.Value() <= -60.0 {
		t.Fatalf("wifi filter tracked too slowly: %v", wf.Value())
	}
}

func TestDistanceEstimationAndReferences(t *testing.T) {
	ref := WifiReferenceAt1m(2437)
	if ref != -38.0 {
		t.Fatalf("2.4 GHz ref = %v, want -38", ref)
	}
	if got := WifiReferenceAt1m(5180); got != -43.0 {
		t.Fatalf("5 GHz ref = %v, want -43", got)
	}
	if got := BleReferenceAt1m(0, true); got != -41.0 {
		t.Fatalf("BLE tx=0 ref = %v, want -41", got)
	}
	if got := BleReferenceAt1m(0, false); got != -62.0 {
		t.Fatalf("BLE no-tx ref = %v, want -62", got)
	}

	near := EstimateDistance(-45.0, ref)
	far := EstimateDistance(-75.0, ref)
	if !(near.MinMeters <= near.TypicalMeters && near.TypicalMeters <= near.MaxMeters) {
		t.Fatalf("near range unordered: %+v", near)
	}
	if far.TypicalMeters <= near.TypicalMeters {
		t.Fatalf("far (%v) should exceed near (%v)", far.TypicalMeters, near.TypicalMeters)
	}
	at1m := EstimateDistance(ref, ref)
	if math.Abs(at1m.TypicalMeters-1.0) > 0.01 {
		t.Fatalf("at 1m ref got %v", at1m.TypicalMeters)
	}

	veryClose := EstimateDistance(-30.0, -41.0)
	if veryClose.MinMeters > veryClose.MaxMeters || veryClose.MaxMeters >= 1.0 {
		t.Fatalf("very close range invalid: %+v", veryClose)
	}
}

func TestTrendDetector(t *testing.T) {
	tr := NewTrendDetector(3000, 2.0)
	var tm int64
	last := TrendSteady
	for _, rssi := range []float64{-70, -69, -67, -65, -63} {
		last = tr.Update(tm, rssi)
		tm += 1000
	}
	if last != TrendCloser {
		t.Fatalf("trend = %v, want CLOSER", last)
	}

	noise := NewTrendDetector(0, 0)
	tm = 0
	r := TrendCloser
	for _, rssi := range []float64{-60.0, -60.5, -59.6, -60.3, -59.9, -60.2} {
		r = noise.Update(tm, rssi)
		tm += 1000
	}
	if r != TrendSteady {
		t.Fatalf("noisy trend = %v, want STEADY", r)
	}
	noise.Reset()

	sparse := NewTrendDetector(8000, 2.0)
	sparse.Update(0, -80)
	sparse.Update(3000, -78)
	if got := sparse.Update(9000, -84); got != TrendFarther {
		t.Fatalf("sparse trend = %v, want FARTHER", got)
	}
}

func makeTurnSamples(targetDeg, swingDB, noiseDB float64, perSector int, seed int64) []AzimuthRssi {
	rng := rand.New(rand.NewSource(seed))
	out := make([]AzimuthRssi, 0, BearingSectors*perSector)
	for sector := 0; sector < BearingSectors; sector++ {
		for i := 0; i < perSector; i++ {
			az := float64(sector)*45.0 + (float64(i)+0.5)*45.0/float64(perSector)
			sig := -65.0 + (swingDB/2.0)*math.Cos((az-targetDeg)*math.Pi/180.0) + rng.NormFloat64()*noiseDB
			out = append(out, AzimuthRssi{AzimuthDeg: az, Rssi: sig})
		}
	}
	return out
}

func TestBearingEstimator(t *testing.T) {
	targets := []struct {
		deg  float64
		seed int64
	}{
		{120, 1},
		{10, 2},
		{275, 3},
		{350, 9},
	}
	for _, tc := range targets {
		est, ok := EstimateBearing(makeTurnSamples(tc.deg, 8.0, 3.0, 6, tc.seed), 2)
		if !ok {
			t.Fatalf("target %v: expected estimate", tc.deg)
		}
		if errDeg := AngleErrorDeg(est.BearingDeg, tc.deg); errDeg >= 30.0 {
			t.Fatalf("target %v: got %v (err=%v)", tc.deg, est.BearingDeg, errDeg)
		}
		if est.Confidence == BearingConfidenceInconclusive {
			t.Fatalf("target %v: unexpected INCONCLUSIVE (%+v)", tc.deg, est)
		}
	}

	flat, ok := EstimateBearing(makeTurnSamples(0, 0, 4.0, 5, 4), 2)
	if !ok || flat.Confidence != BearingConfidenceInconclusive {
		t.Fatalf("zero swing: got (%+v, %v), want INCONCLUSIVE", flat, ok)
	}

	full := makeTurnSamples(90, 8.0, 2.0, 3, 5)
	incomplete := make([]AzimuthRssi, 0, len(full))
	for _, s := range full {
		if SectorOf(s.AzimuthDeg) != 6 {
			incomplete = append(incomplete, s)
		}
	}
	if _, ok := EstimateBearing(incomplete, 2); ok {
		t.Fatal("incomplete turn must not produce estimate")
	}
	if SectorCounts(incomplete)[6] != 0 {
		t.Fatal("expected sector 6 count == 0")
	}

	// Lingering heavily in one direction must not bias sector-weighted fit.
	base := makeTurnSamples(200, 8.0, 3.0, 3, 6)
	for i := 0; i < 40; i++ {
		sig := -65.0 + 4.0*math.Cos((100.0-200.0)*math.Pi/180.0)
		base = append(base, AzimuthRssi{AzimuthDeg: 100.0, Rssi: sig})
	}
	est, ok := EstimateBearing(base, 2)
	if !ok || AngleErrorDeg(est.BearingDeg, 200.0) >= 30.0 {
		t.Fatalf("biased sampling gave (%+v, %v)", est, ok)
	}
}

func TestGeigerAndMacSibling(t *testing.T) {
	weak := GeigerIntervalMillis(-100)
	mid := GeigerIntervalMillis(-65)
	strong := GeigerIntervalMillis(-30)
	if weak != GeigerSlowestMillis || strong != GeigerFastestMillis {
		t.Fatalf("bounds = (%d, %d), want (%d, %d)", weak, strong, GeigerSlowestMillis, GeigerFastestMillis)
	}
	if !(strong < mid && mid < weak) {
		t.Fatalf("expected strictly monotonic intervals, got %d < %d < %d", strong, mid, weak)
	}

	netDevs := []MacDevice{
		{MAC: "02:00:00:12:34:10", Name: "SmartStrip"},
		{MAC: "02:00:00:af:91:42", Name: "LivingRoomTV"},
	}
	if got := MacSiblingName("02:00:00:12:34:12", netDevs, 3); got != "SmartStrip" {
		t.Fatalf("sibling = %q, want SmartStrip", got)
	}
	if got := MacSiblingName("02:00:00:12:35:12", netDevs, 3); got != "" {
		t.Fatalf("different OUI/octet5 = %q, want empty", got)
	}
	if got := MacSiblingName("not-a-mac", netDevs, 3); got != "" {
		t.Fatalf("invalid mac = %q, want empty", got)
	}

	ambig := []MacDevice{
		{MAC: "02:00:00:12:34:10", Name: "Bulb 1"},
		{MAC: "02:00:00:12:34:13", Name: "Bulb 2"},
	}
	if got := MacSiblingName("02:00:00:12:34:11", ambig, 3); got != "" {
		t.Fatalf("ambiguous match = %q, want empty", got)
	}
}

func TestRadarConcurrencyRaceSafe(t *testing.T) {
	f := NewBleRssiFilter()
	td := NewTrendDetector(1000, 2.0)
	bt := NewBearingTracker()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				v := f.Update(float64(-60 - (id+i)%15))
				_ = td.Update(int64(i*200), v)
				bt.AddSample(float64((id*45+i*7)%360), v)
				_, _ = bt.Estimate(1)
			}
		}(g)
	}
	wg.Wait()
	if bt.SampleCount() != 200 {
		t.Fatalf("SampleCount() = %d, want 200", bt.SampleCount())
	}
	bt.Reset()
	if bt.SampleCount() != 0 {
		t.Fatalf("SampleCount() after Reset = %d, want 0", bt.SampleCount())
	}
}
