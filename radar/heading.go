package radar

import "math"

// Axis constants matching android.hardware.SensorManager.AXIS_* for screen remapping.
const (
	AxisX      = 1
	AxisY      = 2
	AxisMinusX = 129
	AxisMinusY = 130
)

// NormalizeDegrees wraps deg into the half-open interval [0, 360).
func NormalizeDegrees(deg float64) float64 {
	r := math.Mod(deg, 360.0)
	if r < 0 {
		r += 360.0
	}
	if r == 360.0 || r == -0.0 {
		return 0
	}
	return r
}

// AngleErrorDeg returns the absolute angular separation between a and b in [0, 180].
func AngleErrorDeg(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360.0)
	if d > 180.0 {
		return 360.0 - d
	}
	return d
}

// ShortestAngleDiff returns the signed smallest turn in (-180, 180] from fromDeg to toDeg.
func ShortestAngleDiff(fromDeg, toDeg float64) float64 {
	d := math.Mod(toDeg-fromDeg+540.0, 360.0) - 180.0
	if d == -180.0 {
		return 180.0
	}
	return d
}

func headingFromAxis(r [9]float64, eastIndex, northIndex int, negate bool) float64 {
	sign := 1.0
	if negate {
		sign = -1.0
	}
	east := sign * r[eastIndex]
	north := sign * r[northIndex]
	return NormalizeDegrees(math.Atan2(east, north) * 180.0 / math.Pi)
}

// FlatAzimuthDeg computes device top-edge compass azimuth from a 3x3 rotation matrix.
func FlatAzimuthDeg(r [9]float64) float64 {
	return headingFromAxis(r, 1, 4, false)
}

// VerticalAzimuthDeg computes rear-camera compass azimuth for upright posture.
func VerticalAzimuthDeg(r [9]float64) float64 {
	return headingFromAxis(r, 2, 5, true)
}

// PitchDeg extracts device pitch in degrees (asin(-R[7])).
func PitchDeg(r [9]float64) float64 {
	return math.Asin(clamp(-r[7], -1.0, 1.0)) * 180.0 / math.Pi
}

// PostureAwareAzimuthDeg selects VerticalAzimuthDeg when |pitch| > verticalThresholdDeg,
// otherwise FlatAzimuthDeg. If verticalThresholdDeg <= 0, 45 is used.
func PostureAwareAzimuthDeg(r [9]float64, verticalThresholdDeg float64) float64 {
	if verticalThresholdDeg <= 0 {
		verticalThresholdDeg = 45.0
	}
	if math.Abs(PitchDeg(r)) > verticalThresholdDeg {
		return VerticalAzimuthDeg(r)
	}
	return FlatAzimuthDeg(r)
}

// SmoothAzimuthDeg applies circular exponential smoothing on unit vectors so
// wrapping across 359 -> 0 never jumps through 180.
func SmoothAzimuthDeg(previousDeg float64, hasPrevious bool, nextDeg, alpha float64) float64 {
	if !hasPrevious {
		return NormalizeDegrees(nextDeg)
	}
	prevRad := previousDeg * math.Pi / 180.0
	nextRad := nextDeg * math.Pi / 180.0
	x := (1.0-alpha)*math.Cos(prevRad) + alpha*math.Cos(nextRad)
	y := (1.0-alpha)*math.Sin(prevRad) + alpha*math.Sin(nextRad)
	return NormalizeDegrees(math.Atan2(y, x) * 180.0 / math.Pi)
}

// ScreenRemapAxes returns the (X, Y) remapped axis pair for display rotations 0..3.
func ScreenRemapAxes(surfaceRotation int) (int, int) {
	switch surfaceRotation {
	case 1:
		return AxisY, AxisMinusX
	case 2:
		return AxisMinusX, AxisMinusY
	case 3:
		return AxisMinusY, AxisX
	default:
		return AxisX, AxisY
	}
}
