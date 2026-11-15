package mobile

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/core"
)

// NetworkKey builds the canonical "<gateway>|<subnet>" key used by agent/history.
func NetworkKey(gateway, subnet string) string {
	return string(history.NewNetworkKey(gateway, subnet))
}

// StrongNameOf extracts the canonical lowercase reconciliation name for a core.Device JSON object.
func StrongNameOf(deviceJSON string) (string, error) {
	var d core.Device
	if err := json.Unmarshal([]byte(deviceJSON), &d); err != nil {
		return "", err
	}
	return history.StrongNameOf(d), nil
}

// ReconcileDevices matches scanned devices JSON ([]core.Device) against known
// devices JSON ([]history.KnownDevice) and returns []history.Match as JSON.
func ReconcileDevices(knownJSON, scannedJSON string) (string, error) {
	var known []history.KnownDevice
	if strings.TrimSpace(knownJSON) != "" {
		if err := json.Unmarshal([]byte(knownJSON), &known); err != nil {
			return "", err
		}
	}
	var scanned []core.Device
	if strings.TrimSpace(scannedJSON) != "" {
		if err := json.Unmarshal([]byte(scannedJSON), &scanned); err != nil {
			return "", err
		}
	}
	matches := history.Reconcile(known, scanned)
	out, err := json.Marshal(matches)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func timestampFromMillis(unixMillis int64) time.Time {
	if unixMillis <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(unixMillis).UTC()
}

// HistorySeen transitions a history.KnownDevice JSON object after being observed
// in a scan and returns the resulting history.Outcome as JSON.
func HistorySeen(previousJSON, scannedJSON string, atUnixMillis int64) (string, error) {
	var prev history.KnownDevice
	if err := json.Unmarshal([]byte(previousJSON), &prev); err != nil {
		return "", err
	}
	var scanned core.Device
	if err := json.Unmarshal([]byte(scannedJSON), &scanned); err != nil {
		return "", err
	}
	outcome := history.Seen(prev, scanned, timestampFromMillis(atUnixMillis))
	out, err := json.Marshal(outcome)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// HistoryMissing transitions a history.KnownDevice JSON object after missing a
// scan and returns the resulting history.Outcome as JSON (including grace accounting).
func HistoryMissing(previousJSON string, offlineMissesBefore int, atUnixMillis int64) (string, error) {
	var prev history.KnownDevice
	if err := json.Unmarshal([]byte(previousJSON), &prev); err != nil {
		return "", err
	}
	outcome := history.Missing(prev, offlineMissesBefore, timestampFromMillis(atUnixMillis))
	out, err := json.Marshal(outcome)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
