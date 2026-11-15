package radar

import (
	"math"
	"strconv"
	"strings"
	"sync"
)

// RssiFilter is a 1-D Kalman filter that smooths noisy RSSI readings without
// the lag of a long moving average. It is safe for concurrent use.
type RssiFilter struct {
	mu               sync.Mutex
	processNoise     float64
	measurementNoise float64
	hasEstimate      bool
	estimate         float64
	errorCovariance  float64
}

// NewRssiFilter constructs a Kalman RSSI filter with custom noise parameters.
func NewRssiFilter(processNoise, measurementNoise float64) *RssiFilter {
	if processNoise <= 0 {
		processNoise = 0.1
	}
	if measurementNoise <= 0 {
		measurementNoise = 9.0
	}
	return &RssiFilter{
		processNoise:     processNoise,
		measurementNoise: measurementNoise,
	}
}

// NewBleRssiFilter returns a Kalman filter tuned for high-rate noisy BLE readings.
func NewBleRssiFilter() *RssiFilter {
	return NewRssiFilter(0.1, 9.0)
}

// NewWifiRssiFilter returns a Kalman filter tuned for slower Wi-Fi RSSI polling.
func NewWifiRssiFilter() *RssiFilter {
	return NewRssiFilter(1.5, 9.0)
}

// HasValue reports whether at least one RSSI reading has been fed into the filter.
func (f *RssiFilter) HasValue() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hasEstimate
}

// ValueOK returns the current smoothed RSSI and whether one has been computed.
func (f *RssiFilter) ValueOK() (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.estimate, f.hasEstimate
}

// Value returns the current smoothed RSSI, or 0 before the first reading.
func (f *RssiFilter) Value() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasEstimate {
		return 0
	}
	return f.estimate
}

// Update incorporates a raw RSSI measurement and returns the updated estimate.
func (f *RssiFilter) Update(rssi float64) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasEstimate {
		f.estimate = rssi
		f.hasEstimate = true
		f.errorCovariance = f.measurementNoise
		return rssi
	}
	predictedCov := f.errorCovariance + f.processNoise
	gain := predictedCov / (predictedCov + f.measurementNoise)
	next := f.estimate + gain*(rssi-f.estimate)
	f.errorCovariance = (1 - gain) * predictedCov
	f.estimate = next
	return next
}

// Reset clears the filter state back to uninitialized.
func (f *RssiFilter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hasEstimate = false
	f.estimate = 0
	f.errorCovariance = 0
}

// DistanceRange represents an estimated distance band in meters.
type DistanceRange struct {
	MinMeters     float64 `json:"min_meters"`
	TypicalMeters float64 `json:"typical_meters"`
	MaxMeters     float64 `json:"max_meters"`
}

const (
	PathLossMin       = 2.0
	PathLossTypical   = 2.7
	PathLossMax       = 3.5
	MinDistanceMeters = 0.3
	MaxDistanceMeters = 60.0
)

// WifiReferenceAt1m returns the expected Wi-Fi RSSI at 1 m for frequencyMHz.
func WifiReferenceAt1m(frequencyMHz int) float64 {
	if frequencyMHz >= 4900 {
		return -43.0
	}
	return -38.0
}

// BleReferenceAt1m returns the expected BLE RSSI at 1 m given an optional advertised Tx power.
func BleReferenceAt1m(txPowerDBm int, hasTxPower bool) float64 {
	if hasTxPower {
		return float64(txPowerDBm) - 41.0
	}
	return -62.0
}

// EstimateDistance estimates a [min, typical, max] distance range in meters
// using the log-distance path-loss model: rssi = referenceAt1m - 10*n*log10(d).
func EstimateDistance(rssi, referenceAt1m float64) DistanceRange {
	calc := func(n float64) float64 {
		d := math.Pow(10.0, (referenceAt1m-rssi)/(10.0*n))
		return clamp(d, MinDistanceMeters, MaxDistanceMeters)
	}
	a := calc(PathLossMax)
	b := calc(PathLossMin)
	if a > b {
		a, b = b, a
	}
	return DistanceRange{
		MinMeters:     a,
		TypicalMeters: calc(PathLossTypical),
		MaxMeters:     b,
	}
}

// ProximityFraction normalizes a smoothed RSSI in [-95, -35] dBm to [0, 1].
func ProximityFraction(smoothedRssi float64) float64 {
	return clamp((smoothedRssi+95.0)/60.0, 0.0, 1.0)
}

// Trend indicates whether the signal is getting stronger, weaker, or holding steady.
type Trend string

const (
	TrendCloser  Trend = "CLOSER"
	TrendFarther Trend = "FARTHER"
	TrendSteady  Trend = "STEADY"
)

type trendSample struct {
	atMillis int64
	rssi     float64
}

// TrendDetector compares current smoothed RSSI against a sample from windowMillis ago.
type TrendDetector struct {
	mu           sync.Mutex
	windowMillis int64
	thresholdDB  float64
	history      []trendSample
}

// NewTrendDetector constructs a TrendDetector. Defaults windowMillis=3000 and
// thresholdDB=2.0 when non-positive values are passed.
func NewTrendDetector(windowMillis int64, thresholdDB float64) *TrendDetector {
	if windowMillis <= 0 {
		windowMillis = 3000
	}
	if thresholdDB <= 0 {
		thresholdDB = 2.0
	}
	return &TrendDetector{
		windowMillis: windowMillis,
		thresholdDB:  thresholdDB,
	}
}

// Update records a smoothed RSSI reading at atMillis and returns the resulting Trend.
func (t *TrendDetector) Update(atMillis int64, smoothedRssi float64) Trend {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history = append(t.history, trendSample{atMillis: atMillis, rssi: smoothedRssi})
	for len(t.history) > 2 && atMillis-t.history[1].atMillis >= t.windowMillis {
		t.history = t.history[1:]
	}
	ref := t.history[0]
	if atMillis-ref.atMillis < t.windowMillis {
		return TrendSteady
	}
	delta := smoothedRssi - ref.rssi
	switch {
	case delta >= t.thresholdDB:
		return TrendCloser
	case delta <= -t.thresholdDB:
		return TrendFarther
	default:
		return TrendSteady
	}
}

// Reset clears accumulated trend samples.
func (t *TrendDetector) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history = nil
}

// BearingSectors is the number of 45-degree compass sectors for guided turns.
const BearingSectors = 8

const sectorWidth = 360.0 / BearingSectors

// BearingConfidence classifies how trustworthy a guided-turn bearing estimate is.
type BearingConfidence string

const (
	BearingConfidenceHigh         BearingConfidence = "HIGH"
	BearingConfidenceMedium       BearingConfidence = "MEDIUM"
	BearingConfidenceInconclusive BearingConfidence = "INCONCLUSIVE"
)

// AzimuthRssi pairs a compass heading (degrees clockwise from north) with RSSI.
type AzimuthRssi struct {
	AzimuthDeg float64 `json:"azimuth_deg"`
	Rssi       float64 `json:"rssi"`
}

// BearingEstimate holds the fitted cosine maximum direction, front-to-back swing, and confidence.
type BearingEstimate struct {
	BearingDeg float64           `json:"bearing_deg"`
	SwingDB    float64           `json:"swing_db"`
	Confidence BearingConfidence `json:"confidence"`
}

// SectorOf maps azimuthDeg into a sector index in [0, BearingSectors-1].
func SectorOf(azimuthDeg float64) int {
	idx := int(NormalizeDegrees(azimuthDeg) / sectorWidth)
	if idx < 0 {
		return 0
	}
	if idx >= BearingSectors {
		return BearingSectors - 1
	}
	return idx
}

// SectorCounts returns the sample count per 45-degree sector.
func SectorCounts(samples []AzimuthRssi) [BearingSectors]int {
	var counts [BearingSectors]int
	for _, s := range samples {
		counts[SectorOf(s.AzimuthDeg)]++
	}
	return counts
}

// EstimateBearing fits rssi = a + b*cos(theta) + c*sin(theta) via sector-weighted
// least squares. Returns (estimate, false) until every sector has at least minPerSector samples.
func EstimateBearing(samples []AzimuthRssi, minPerSector int) (BearingEstimate, bool) {
	if minPerSector <= 0 {
		minPerSector = 2
	}
	counts := SectorCounts(samples)
	for _, c := range counts {
		if c < minPerSector {
			return BearingEstimate{}, false
		}
	}

	var sw, sc, ss, scc, sss, scs, sr, src, srs float64
	for _, s := range samples {
		w := 1.0 / float64(counts[SectorOf(s.AzimuthDeg)])
		th := s.AzimuthDeg * math.Pi / 180.0
		c := math.Cos(th)
		sn := math.Sin(th)
		sw += w
		sc += w * c
		ss += w * sn
		scc += w * c * c
		sss += w * sn * sn
		scs += w * c * sn
		sr += w * s.Rssi
		src += w * s.Rssi * c
		srs += w * s.Rssi * sn
	}

	sol, ok := solve3(
		[3][3]float64{
			{sw, sc, ss},
			{sc, scc, scs},
			{ss, scs, sss},
		},
		[3]float64{sr, src, srs},
	)
	if !ok {
		return BearingEstimate{}, false
	}
	a, b, c := sol[0], sol[1], sol[2]
	amplitude := math.Hypot(b, c)

	var squaredError float64
	for _, s := range samples {
		w := 1.0 / float64(counts[SectorOf(s.AzimuthDeg)])
		th := s.AzimuthDeg * math.Pi / 180.0
		diff := s.Rssi - (a + b*math.Cos(th) + c*math.Sin(th))
		squaredError += w * diff * diff
	}
	noise := math.Sqrt(squaredError / sw)
	amplitudeError := noise * math.Sqrt(2.0/float64(len(samples)))
	swing := 2.0 * amplitude

	conf := BearingConfidenceInconclusive
	switch {
	case swing >= 6.0 && amplitude >= 4.0*amplitudeError:
		conf = BearingConfidenceHigh
	case swing >= 3.0 && amplitude >= 2.5*amplitudeError:
		conf = BearingConfidenceMedium
	}
	bearingDeg := NormalizeDegrees(math.Atan2(c, b) * 180.0 / math.Pi)
	return BearingEstimate{
		BearingDeg: bearingDeg,
		SwingDB:    swing,
		Confidence: conf,
	}, true
}

func solve3(m [3][3]float64, v [3]float64) ([3]float64, bool) {
	var a [3][4]float64
	for i := 0; i < 3; i++ {
		a[i][0], a[i][1], a[i][2], a[i][3] = m[i][0], m[i][1], m[i][2], v[i]
	}
	for col := 0; col < 3; col++ {
		pivot := col
		maxAbs := math.Abs(a[col][col])
		for row := col + 1; row < 3; row++ {
			if av := math.Abs(a[row][col]); av > maxAbs {
				maxAbs = av
				pivot = row
			}
		}
		if maxAbs < 1e-12 {
			return [3]float64{}, false
		}
		a[col], a[pivot] = a[pivot], a[col]
		for row := col + 1; row < 3; row++ {
			f := a[row][col] / a[col][col]
			for k := col; k <= 3; k++ {
				a[row][k] -= f * a[col][k]
			}
		}
	}
	var x [3]float64
	for i := 2; i >= 0; i-- {
		sum := a[i][3]
		for k := i + 1; k < 3; k++ {
			sum -= a[i][k] * x[k]
		}
		x[i] = sum / a[i][i]
	}
	return x, true
}

// BearingTracker accumulates (azimuth, rssi) samples incrementally for a guided turn.
type BearingTracker struct {
	mu      sync.Mutex
	samples []AzimuthRssi
}

// NewBearingTracker returns an empty BearingTracker.
func NewBearingTracker() *BearingTracker {
	return &BearingTracker{}
}

// AddSample appends an (azimuthDeg, rssi) observation.
func (b *BearingTracker) AddSample(azimuthDeg, rssi float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.samples = append(b.samples, AzimuthRssi{AzimuthDeg: azimuthDeg, Rssi: rssi})
}

// SampleCount returns the number of recorded samples.
func (b *BearingTracker) SampleCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.samples)
}

// SectorCounts returns the per-sector sample histogram across all 8 sectors.
func (b *BearingTracker) SectorCounts() [BearingSectors]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return SectorCounts(b.samples)
}

// SectorCount returns how many samples fall in the given sector [0..7], or 0 if out of range.
func (b *BearingTracker) SectorCount(sector int) int {
	if sector < 0 || sector >= BearingSectors {
		return 0
	}
	counts := b.SectorCounts()
	return counts[sector]
}

// Estimate computes the bearing estimate from all accumulated samples.
func (b *BearingTracker) Estimate(minPerSector int) (BearingEstimate, bool) {
	b.mu.Lock()
	snap := make([]AzimuthRssi, len(b.samples))
	copy(snap, b.samples)
	b.mu.Unlock()
	return EstimateBearing(snap, minPerSector)
}

// Reset clears all accumulated samples.
func (b *BearingTracker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.samples = nil
}

const (
	GeigerSlowestMillis int64 = 1200
	GeigerFastestMillis int64 = 80
)

// GeigerIntervalMillis maps a smoothed RSSI to click period in milliseconds
// on an exponential scale between GeigerSlowestMillis (-95 dBm) and
// GeigerFastestMillis (-35 dBm).
func GeigerIntervalMillis(smoothedRssi float64) int64 {
	t := ProximityFraction(smoothedRssi)
	ratio := float64(GeigerFastestMillis) / float64(GeigerSlowestMillis)
	return int64(float64(GeigerSlowestMillis) * math.Pow(ratio, t))
}

// MacDevice pairs a network MAC address with a device display name.
type MacDevice struct {
	MAC  string `json:"mac"`
	Name string `json:"name"`
}

// MacSiblingName returns the unique network device name whose MAC matches the
// first 5 octets of bleAddress and differs in the 6th octet by at most maxDelta,
// or "" if there is no unique match.
func MacSiblingName(bleAddress string, networkDevices []MacDevice, maxDelta int) string {
	if maxDelta < 0 {
		maxDelta = 3
	}
	ble, ok := parseMACBytes(bleAddress)
	if !ok {
		return ""
	}
	seen := make(map[string]struct{})
	var match string
	for _, dev := range networkDevices {
		wifi, ok := parseMACBytes(dev.MAC)
		if !ok {
			continue
		}
		if wifi[0] != ble[0] || wifi[1] != ble[1] || wifi[2] != ble[2] || wifi[3] != ble[3] || wifi[4] != ble[4] {
			continue
		}
		delta := wifi[5] - ble[5]
		if delta < 0 {
			delta = -delta
		}
		if delta <= maxDelta {
			if _, exists := seen[dev.Name]; !exists {
				seen[dev.Name] = struct{}{}
				match = dev.Name
			}
		}
	}
	if len(seen) != 1 {
		return ""
	}
	return match
}

func parseMACBytes(mac string) ([6]int, bool) {
	var out [6]int
	parts := strings.FieldsFunc(strings.TrimSpace(mac), func(r rune) bool {
		return r == ':' || r == '-'
	})
	if len(parts) != 6 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		v, err := strconv.ParseInt(p, 16, 16)
		if err != nil || v < 0 || v > 255 {
			return out, false
		}
		out[i] = int(v)
	}
	return out, true
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
