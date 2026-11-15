package mobile

import (
	"encoding/json"
	"strings"

	"github.com/baxner/vigia/radar"
)

// BluetoothCompanyCount returns the count of Bluetooth SIG company IDs embedded in the binary.
func BluetoothCompanyCount() int {
	return radar.KnownCompanyCount()
}

// BluetoothCompanyFullName returns the official Bluetooth SIG name for companyID, or "".
func BluetoothCompanyFullName(companyID int) string {
	return radar.CompanyFullName(companyID)
}

// BluetoothCompanyBrand returns a shortened brand display name for companyID, or "".
func BluetoothCompanyBrand(companyID int) string {
	return radar.CompanyBrand(companyID)
}

// BluetoothShortName strips trailing corporate legal suffixes from fullName.
func BluetoothShortName(fullName string) string {
	return radar.ShortCompanyName(fullName)
}

// RssiFilter wraps radar.RssiFilter for gomobile callers (Android/iOS).
type RssiFilter struct {
	inner *radar.RssiFilter
}

// NewRssiFilter creates a custom 1-D Kalman RSSI filter.
func NewRssiFilter(processNoise, measurementNoise float64) *RssiFilter {
	return &RssiFilter{inner: radar.NewRssiFilter(processNoise, measurementNoise)}
}

// NewBleRssiFilter creates a Kalman RSSI filter tuned for BLE RSSI streams.
func NewBleRssiFilter() *RssiFilter {
	return &RssiFilter{inner: radar.NewBleRssiFilter()}
}

// NewWifiRssiFilter creates a Kalman RSSI filter tuned for Wi-Fi RSSI polling.
func NewWifiRssiFilter() *RssiFilter {
	return &RssiFilter{inner: radar.NewWifiRssiFilter()}
}

// HasValue reports whether at least one reading has been processed.
func (f *RssiFilter) HasValue() bool {
	if f == nil || f.inner == nil {
		return false
	}
	return f.inner.HasValue()
}

// Value returns the smoothed RSSI in dBm (or 0 before the first reading).
func (f *RssiFilter) Value() float64 {
	if f == nil || f.inner == nil {
		return 0
	}
	return f.inner.Value()
}

// Update ingests a raw RSSI measurement and returns the smoothed RSSI.
func (f *RssiFilter) Update(rssi float64) float64 {
	if f == nil {
		return rssi
	}
	if f.inner == nil {
		f.inner = radar.NewBleRssiFilter()
	}
	return f.inner.Update(rssi)
}

// Reset clears the filter state back to uninitialized.
func (f *RssiFilter) Reset() {
	if f != nil && f.inner != nil {
		f.inner.Reset()
	}
}

// DistanceRange holds min/typical/max distance estimates in meters.
type DistanceRange struct {
	MinMeters     float64
	TypicalMeters float64
	MaxMeters     float64
}

// EstimateDistance estimates a path-loss distance range for rssi given referenceAt1m.
func EstimateDistance(rssi, referenceAt1m float64) *DistanceRange {
	d := radar.EstimateDistance(rssi, referenceAt1m)
	return &DistanceRange{
		MinMeters:     d.MinMeters,
		TypicalMeters: d.TypicalMeters,
		MaxMeters:     d.MaxMeters,
	}
}

// WifiReferenceAt1m returns the expected Wi-Fi RSSI at 1 m for frequencyMHz.
func WifiReferenceAt1m(frequencyMHz int) float64 {
	return radar.WifiReferenceAt1m(frequencyMHz)
}

// BleReferenceAt1m returns the expected BLE RSSI at 1 m.
func BleReferenceAt1m(txPowerDBm int, hasTxPower bool) float64 {
	return radar.BleReferenceAt1m(txPowerDBm, hasTxPower)
}

// ProximityFraction normalizes a smoothed RSSI in [-95, -35] dBm to [0, 1].
func ProximityFraction(smoothedRssi float64) float64 {
	return radar.ProximityFraction(smoothedRssi)
}

// GeigerIntervalMillis maps a smoothed RSSI to click interval milliseconds.
func GeigerIntervalMillis(smoothedRssi float64) int64 {
	return radar.GeigerIntervalMillis(smoothedRssi)
}

// TrendDetector wraps radar.TrendDetector for gomobile callers.
type TrendDetector struct {
	inner *radar.TrendDetector
}

// NewTrendDetector constructs a signal trend detector. Non-positive parameters use defaults.
func NewTrendDetector(windowMillis int64, thresholdDB float64) *TrendDetector {
	return &TrendDetector{inner: radar.NewTrendDetector(windowMillis, thresholdDB)}
}

// Update records a smoothed RSSI at atMillis and returns "CLOSER", "FARTHER", or "STEADY".
func (t *TrendDetector) Update(atMillis int64, smoothedRssi float64) string {
	if t == nil {
		return string(radar.TrendSteady)
	}
	if t.inner == nil {
		t.inner = radar.NewTrendDetector(0, 0)
	}
	return string(t.inner.Update(atMillis, smoothedRssi))
}

// Reset clears accumulated samples.
func (t *TrendDetector) Reset() {
	if t != nil && t.inner != nil {
		t.inner.Reset()
	}
}

// BearingResult holds the outcome of a guided-turn bearing estimation.
type BearingResult struct {
	Available  bool
	BearingDeg float64
	SwingDB    float64
	Confidence string
}

// BearingEstimator incrementally collects (azimuthDeg, rssi) samples and fits a cosine bearing.
type BearingEstimator struct {
	inner *radar.BearingTracker
}

// NewBearingEstimator creates an empty BearingEstimator.
func NewBearingEstimator() *BearingEstimator {
	return &BearingEstimator{inner: radar.NewBearingTracker()}
}

// AddSample appends a compass heading (degrees) and smoothed/raw RSSI sample.
func (b *BearingEstimator) AddSample(azimuthDeg, rssi float64) {
	if b == nil {
		return
	}
	if b.inner == nil {
		b.inner = radar.NewBearingTracker()
	}
	b.inner.AddSample(azimuthDeg, rssi)
}

// SampleCount returns the total number of samples added.
func (b *BearingEstimator) SampleCount() int {
	if b == nil || b.inner == nil {
		return 0
	}
	return b.inner.SampleCount()
}

// SectorCount returns the sample count for sector in [0..7].
func (b *BearingEstimator) SectorCount(sector int) int {
	if b == nil || b.inner == nil {
		return 0
	}
	return b.inner.SectorCount(sector)
}

// SectorCountsJSON returns the 8-element sector histogram serialized as JSON.
func (b *BearingEstimator) SectorCountsJSON() string {
	var counts [radar.BearingSectors]int
	if b != nil && b.inner != nil {
		counts = b.inner.SectorCounts()
	}
	raw, err := json.Marshal(counts[:])
	if err != nil {
		return "[0,0,0,0,0,0,0,0]"
	}
	return string(raw)
}

// Estimate computes the bearing estimate from collected samples.
func (b *BearingEstimator) Estimate(minPerSector int) *BearingResult {
	if b == nil || b.inner == nil {
		return &BearingResult{Available: false, Confidence: string(radar.BearingConfidenceInconclusive)}
	}
	est, ok := b.inner.Estimate(minPerSector)
	if !ok {
		return &BearingResult{Available: false, Confidence: string(radar.BearingConfidenceInconclusive)}
	}
	return &BearingResult{
		Available:  true,
		BearingDeg: est.BearingDeg,
		SwingDB:    est.SwingDB,
		Confidence: string(est.Confidence),
	}
}

// Reset clears all samples from the estimator.
func (b *BearingEstimator) Reset() {
	if b != nil && b.inner != nil {
		b.inner.Reset()
	}
}

// EstimateBearingJSON fits a cosine bearing on a JSON array of {"azimuth_deg","rssi"}
// samples, returning "null" until all 8 sectors have at least minPerSector readings.
func EstimateBearingJSON(samplesJSON string, minPerSector int) (string, error) {
	var samples []radar.AzimuthRssi
	if strings.TrimSpace(samplesJSON) != "" {
		if err := json.Unmarshal([]byte(samplesJSON), &samples); err != nil {
			return "", err
		}
	}
	est, ok := radar.EstimateBearing(samples, minPerSector)
	if !ok {
		return "null", nil
	}
	raw, err := json.Marshal(est)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// BearingSectorOf maps azimuthDeg into a sector index [0..7].
func BearingSectorOf(azimuthDeg float64) int {
	return radar.SectorOf(azimuthDeg)
}

// LooksLikeRandomBleAddress checks whether a BLE MAC uses random/private address bits.
func LooksLikeRandomBleAddress(address string) bool {
	return radar.LooksLikeRandomAddress(address)
}

// NormalizeDegrees wraps deg into [0, 360).
func NormalizeDegrees(deg float64) float64 {
	return radar.NormalizeDegrees(deg)
}

// ShortestAngleDiff returns the signed smallest turn in (-180, 180] from fromDeg to toDeg.
func ShortestAngleDiff(fromDeg, toDeg float64) float64 {
	return radar.ShortestAngleDiff(fromDeg, toDeg)
}

// SmoothAzimuthDeg applies circular exponential smoothing across 359 -> 0 wraparound.
func SmoothAzimuthDeg(previousDeg float64, hasPrevious bool, nextDeg, alpha float64) float64 {
	return radar.SmoothAzimuthDeg(previousDeg, hasPrevious, nextDeg, alpha)
}

// MacSiblingName resolves a unique sibling LAN device name for bleAddress from a
// JSON array of {"mac","name"} objects, or returns "" if none/ambiguous.
func MacSiblingName(bleAddress, devicesJSON string, maxDelta int) (string, error) {
	var devs []radar.MacDevice
	if strings.TrimSpace(devicesJSON) != "" {
		if err := json.Unmarshal([]byte(devicesJSON), &devs); err != nil {
			return "", err
		}
	}
	return radar.MacSiblingName(bleAddress, devs, maxDelta), nil
}
