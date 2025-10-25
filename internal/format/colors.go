package format

import (
	"fmt"
	"os"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

var colorsEnabled = true

func init() {
	// Disable colors if NO_COLOR env var is set or not a TTY
	if os.Getenv("NO_COLOR") != "" {
		colorsEnabled = false
	}
}

// DisableColors disables colored output
func DisableColors() {
	colorsEnabled = false
}

// EnableColors enables colored output
func EnableColors() {
	colorsEnabled = true
}

// Success returns text in green (for positive states)
func Success(text string) string {
	if !colorsEnabled {
		return text
	}
	return colorGreen + text + colorReset
}

// Warning returns text in yellow (for warnings/cautions)
func Warning(text string) string {
	if !colorsEnabled {
		return text
	}
	return colorYellow + text + colorReset
}

// Error returns text in red (for errors/faults)
func Error(text string) string {
	if !colorsEnabled {
		return text
	}
	return colorRed + text + colorReset
}

// Info returns text in blue (for informational messages)
func Info(text string) string {
	if !colorsEnabled {
		return text
	}
	return colorBlue + text + colorReset
}

// Dim returns text in gray (for less important info)
func Dim(text string) string {
	if !colorsEnabled {
		return text
	}
	return colorGray + text + colorReset
}

// ColorizeValue colors a value based on its semantic meaning
func ColorizeValue(value, expectedGood string) string {
	if value == expectedGood {
		return Success(value)
	}
	return value
}

// ColorizeState colors vehicle/battery states appropriately
func ColorizeState(state string) string {
	switch state {
	case "ready-to-drive", "on", "ideal", "active", "ok", "true", "enabled", "armed":
		return Success(state)
	case "stand-by", "parked", "off", "disabled", "disarmed", "false":
		return Info(state)
	case "shutting-down", "init", "waiting", "delay-armed":
		return Warning(state)
	case "error", "fault", "over-temperature", "under-temperature", "critical":
		return Error(state)
	default:
		return state
	}
}

// ColorizePercentage colors percentage values based on thresholds
func ColorizePercentage(value int) string {
	text := fmt.Sprintf("%d%%", value)
	if value >= 80 {
		return Success(text)
	} else if value >= 40 {
		return text
	} else if value >= 20 {
		return Warning(text)
	}
	return Error(text)
}

// ColorizeTemperature colors temperature values based on ranges
func ColorizeTemperature(tempC int) string {
	text := fmt.Sprintf("%d°C", tempC)
	if tempC < 0 {
		return Info(text) // Cold but not critical
	} else if tempC <= 45 {
		return Success(text) // Normal range
	} else if tempC <= 55 {
		return Warning(text) // Getting warm
	}
	return Error(text) // Too hot
}
