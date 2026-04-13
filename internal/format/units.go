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
