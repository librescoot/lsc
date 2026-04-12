package ota

import (
	"fmt"
	"strconv"
	"time"
)

const requiredStandbyDuration = 3 * time.Minute

// parseVehicleTimestamp parses the vehicle state:timestamp field.
// Handles both Unix milliseconds (redis-ipc <= v0.10) and RFC3339 (redis-ipc >= v0.11).
func parseVehicleTimestamp(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t, true
	}
	if ms, err := strconv.ParseInt(ts, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms), true
	}
	return time.Time{}, false
}

func formatStandbyDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// standbyTimerSummary returns a short string describing the standby timer state,
// or an empty string if no useful info is available.
func standbyTimerSummary(vehicleState, vehicleTimestamp string) string {
	if vehicleState != "stand-by" {
		return "waiting for stand-by"
	}
	t, ok := parseVehicleTimestamp(vehicleTimestamp)
	if !ok {
		return ""
	}
	elapsed := time.Since(t)
	if elapsed >= requiredStandbyDuration {
		return fmt.Sprintf("standby %s, rebooting soon", formatStandbyDuration(elapsed))
	}
	remaining := requiredStandbyDuration - elapsed
	return fmt.Sprintf("standby %s / %s, reboot in %s",
		formatStandbyDuration(elapsed),
		formatStandbyDuration(requiredStandbyDuration),
		formatStandbyDuration(remaining))
}
