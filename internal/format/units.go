package format

import (
	"fmt"
	"strconv"
)

// MillivoltsToVolts converts millivolts string to volts with 1 decimal
func MillivoltsToVolts(mv string) string {
	val, err := strconv.Atoi(mv)
	if err != nil || val == 0 {
		return "0.0 V"
	}
	return fmt.Sprintf("%.1f V", float64(val)/1000.0)
}

// MilliampsToAmps converts milliamps string to amps with 1 decimal
func MilliampsToAmps(ma string) string {
	val, err := strconv.Atoi(ma)
	if err != nil || val == 0 {
		return "0.0 A"
	}
	return fmt.Sprintf("%.1f A", float64(val)/1000.0)
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

// ParseInt safely parses an integer string, returning 0 on error
func ParseInt(s string) int {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

// FormatVoltageColored formats voltage with appropriate coloring
func FormatVoltageColored(mv string) string {
	val := ParseInt(mv)
	text := MillivoltsToVolts(mv)

	// Battery voltage ranges (for 14S lithium: 42-58.8V)
	if val >= 50000 {
		return Success(text) // Good voltage
	} else if val >= 45000 {
		return Warning(text) // Low voltage
	} else if val > 0 {
		return Error(text) // Critical voltage
	}
	return Dim(text)
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
