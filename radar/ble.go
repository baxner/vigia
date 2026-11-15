package radar

import (
	"sort"
	"strconv"
	"strings"
)

// SignalSourceState describes the availability state of a BLE or Wi-Fi signal source.
type SignalSourceState string

const (
	SignalStateOK            SignalSourceState = "OK"
	SignalStateNoPermission  SignalSourceState = "NO_PERMISSION"
	SignalStateRadioOff      SignalSourceState = "RADIO_OFF"
	SignalStateNotFound      SignalSourceState = "NOT_FOUND"
	SignalStateSlowThrottled SignalSourceState = "SLOW_THROTTLED"
)

// BleDevice represents a Bluetooth Low Energy advertisement reading.
type BleDevice struct {
	Address        string `json:"address"`
	Name           string `json:"name,omitempty"`
	Rssi           int    `json:"rssi"`
	TxPower        int    `json:"tx_power,omitempty"`
	HasTxPower     bool   `json:"has_tx_power,omitempty"`
	CompanyID      int    `json:"company_id,omitempty"`
	HasCompanyID   bool   `json:"has_company_id,omitempty"`
	LastSeenMillis int64  `json:"last_seen_millis"`
}

// LooksLikeRandomAddress applies the Bluetooth Core Spec (§1.3) top-2-bits
// heuristic to classify static (11) and resolvable private (01) BLE addresses.
func LooksLikeRandomAddress(address string) bool {
	s := strings.TrimSpace(address)
	if idx := strings.IndexByte(s, ':'); idx >= 0 {
		s = s[:idx]
	} else if idx := strings.IndexByte(s, '-'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	firstByte, err := strconv.ParseInt(s, 16, 16)
	if err != nil {
		return false
	}
	topBits := firstByte & 0xC0
	return topBits == 0xC0 || topBits == 0x40
}

// AggregateBleDevices replaces any existing entry for reading.Address and returns
// the slice sorted from most recently seen to oldest.
func AggregateBleDevices(current []BleDevice, reading BleDevice) []BleDevice {
	out := make([]BleDevice, 0, len(current)+1)
	for _, dev := range current {
		if dev.Address != reading.Address {
			out = append(out, dev)
		}
	}
	out = append(out, reading)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastSeenMillis > out[j].LastSeenMillis
	})
	return out
}

// BleScanState computes the BLE scanner source state from permission and radio flags.
func BleScanState(hasPermission, radioOn bool) SignalSourceState {
	switch {
	case !hasPermission:
		return SignalStateNoPermission
	case !radioOn:
		return SignalStateRadioOff
	default:
		return SignalStateOK
	}
}

// BssidEquals compares two BSSID strings case-insensitively.
func BssidEquals(a, b string) bool {
	if a == "" {
		return false
	}
	return strings.EqualFold(a, b)
}

// WifiConnectedState computes the signal source state for a connected Wi-Fi AP.
func WifiConnectedState(hasPermission, radioOn, connectedToTarget bool) SignalSourceState {
	switch {
	case !hasPermission:
		return SignalStateNoPermission
	case !radioOn:
		return SignalStateRadioOff
	case !connectedToTarget:
		return SignalStateNotFound
	default:
		return SignalStateOK
	}
}

// WifiScanState computes the signal source state for an actively scanned Wi-Fi AP.
func WifiScanState(hasPermission, radioOn, foundThisScan, throttled bool, consecutiveMisses, missThreshold int) SignalSourceState {
	if missThreshold <= 0 {
		missThreshold = 3
	}
	switch {
	case !hasPermission:
		return SignalStateNoPermission
	case !radioOn:
		return SignalStateRadioOff
	case foundThisScan:
		return SignalStateOK
	case throttled:
		return SignalStateSlowThrottled
	case consecutiveMisses >= missThreshold:
		return SignalStateNotFound
	default:
		return SignalStateOK
	}
}
