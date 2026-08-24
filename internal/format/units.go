package format

import (
	"fmt"
	"strconv"
)

// MilliampsToAmps converts milliamps string to a human-readable string (mA if < 1A, A otherwise)
func MilliampsToAmps(ma string) string {
	val, err := strconv.Atoi(ma)
	if err != nil {
		return "0 mA"
	}

	absVal := val
	if absVal < 0 {
		absVal = -absVal
	}

	if absVal < 1000 {
		return fmt.Sprintf("%d mA", val)
	}
	return fmt.Sprintf("%.1f A", float64(val)/1000.0)
}

// MillivoltsToVolts converts millivolts string to a human-readable string (mV if < 1V, V otherwise)
func MillivoltsToVolts(mv string) string {
	val, err := strconv.Atoi(mv)
	if err != nil {
		return "0 mV"
	}

	absVal := val
	if absVal < 0 {
		absVal = -absVal
	}

	if absVal < 1000 {
		return fmt.Sprintf("%d mV", val)
	}
	return fmt.Sprintf("%.1f V", float64(val)/1000.0)
}

// MetersToKilometers converts meters string to kilometers with 1 decimal
func MetersToKilometers(m string) string {
	val, err := strconv.Atoi(m)
	if err != nil || val == 0 {
		return "0.0 km"
	}
	return fmt.Sprintf("%.1f km", float64(val)/1000.0)
}

// FormatPercentage formats a percentage string with % symbol
func FormatPercentage(pct string) string {
	if pct == "" || pct == "0" {
		return "0%"
	}
	return pct + "%"
}

// FormatTemperature formats temperature with °C symbol
func FormatTemperature(tempC string) string {
	if tempC == "" || tempC == "0" {
		return "0°C"
	}
	return tempC + "°C"
}

// FormatSpeed formats speed with km/h
func FormatSpeed(speed string) string {
	if speed == "" || speed == "0" {
		return "0 km/h"
	}
	return speed + " km/h"
}

// FormatRPM formats RPM
func FormatRPM(rpm string) string {
	if rpm == "" || rpm == "0" {
		return "0 RPM"
	}
	return rpm + " RPM"
}

// Bytes renders a byte count in decimal SI units, so kB/MB/GB mean what they
// say. It used to divide by 1024 while still labelling the result MB, which
// reported a 202,055,680 byte tile archive as "192.7 MB".
//
// Decimal also matches the other side of every comparison a reader makes here:
// scootui-qt's map info screen, the sizes the tile repos publish, and the
// release manifest. GitHub's own UI divides by 1024 and labels it MB, so its
// rendering of the same asset reads about 5% smaller than this does. That is
// GitHub being wrong, not a discrepancy to chase.
func Bytes(n int64) string {
	const (
		kB = 1000
		MB = 1000 * kB
		GB = 1000 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= kB:
		return fmt.Sprintf("%.1f kB", float64(n)/kB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ParseInt safely parses an integer string, returning 0 on error
func ParseInt(s string) int {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

// colorizeVoltage applies green/yellow/red coloring to a pre-formatted voltage string.
// goodMin and warnMin are the lower bounds (same unit as val) for green and yellow respectively.
func colorizeVoltage(val int, text string, goodMin, warnMin int) string {
	if val >= goodMin {
		return Success(text)
	} else if val >= warnMin {
		return Warning(text)
	} else if val > 0 {
		return Error(text)
	}
	return Dim(text)
}

// MicrovoltsToVolts converts microvolts string to a human-readable string
func MicrovoltsToVolts(uv string) string {
	val, err := strconv.Atoi(uv)
	if err != nil {
		return "0 V"
	}
	return fmt.Sprintf("%.3f V", float64(val)/1000000.0)
}

// MicroampsToAmps converts microamps string to a human-readable string (mA or A)
func MicroampsToAmps(ua string) string {
	val, err := strconv.Atoi(ua)
	if err != nil {
		return "0 mA"
	}
	absVal := val
	if absVal < 0 {
		absVal = -absVal
	}
	if absVal < 1000000 {
		return fmt.Sprintf("%.1f mA", float64(val)/1000.0)
	}
	return fmt.Sprintf("%.1f A", float64(val)/1000000.0)
}

// MicrowattHoursToWattHours converts a microwatt-hours string to a human-readable Wh string
func MicrowattHoursToWattHours(uwh string) string {
	val, err := strconv.Atoi(uwh)
	if err != nil || val <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1f Wh", float64(val)/1000000.0)
}

// SecondsToHuman converts a seconds string to a compact duration like "2d 4h", "3h 12m", "45m"
func SecondsToHuman(s string) string {
	val, err := strconv.Atoi(s)
	if err != nil || val <= 0 {
		return ""
	}
	days := val / 86400
	hours := (val % 86400) / 3600
	minutes := (val % 3600) / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", val)
	}
}

// FormatVoltageColored formats voltage with appropriate coloring (14S lithium pack, mV)
func FormatVoltageColored(mv string) string {
	// 14S lithium: ~42-58.8V; good >= 50V, warning >= 45V
	return colorizeVoltage(ParseInt(mv), MillivoltsToVolts(mv), 50000, 45000)
}

// FormatAuxVoltageColored formats aux battery voltage with appropriate coloring (~12V nominal, mV)
func FormatAuxVoltageColored(mv string) string {
	return colorizeVoltage(ParseInt(mv), MillivoltsToVolts(mv), 12400, 12000)
}

// FormatCBVoltageColored formats CB battery cell voltage with appropriate coloring (single 21700 cell, µV)
func FormatCBVoltageColored(uv string) string {
	// Single 21700 cell: 3.0-4.2V range; good >= 3.8V, warning >= 3.4V
	return colorizeVoltage(ParseInt(uv), MicrovoltsToVolts(uv), 3800000, 3400000)
}

// FormatChargeColored formats charge percentage with coloring
func FormatChargeColored(charge string) string {
	val := ParseInt(charge)
	return ColorizePercentage(val)
}

// FormatTemperatureColored formats temperature with coloring
func FormatTemperatureColored(temp string) string {
	val := ParseInt(temp)
	return ColorizeTemperature(val)
}

// FormatAmbientTemperatureColored formats a float-valued ambient temperature (°C) with coloring
func FormatAmbientTemperatureColored(temp string) string {
	if temp == "" {
		return Dim("N/A")
	}
	val, err := strconv.ParseFloat(temp, 64)
	if err != nil {
		return Dim("N/A")
	}
	text := fmt.Sprintf("%.1f°C", val)
	switch {
	case val < 0:
		return Info(text)
	case val <= 45:
		return Success(text)
	case val <= 55:
		return Warning(text)
	default:
		return Error(text)
	}
}
