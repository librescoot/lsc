package format

import "strings"

// FaultSeries is the letter a component's fault codes carry when codes from
// different components are shown together.
//
// The handbook names the B-series (battery) and the E-series (motor
// controller); the vehicle service's own codes are not part of a named series,
// so they carry a V to keep the group readable next to the others.
type FaultSeries string

const (
	SeriesVehicle FaultSeries = "V"
	SeriesMotor   FaultSeries = "E"
	SeriesBattery FaultSeries = "B"
)

// FaultLabel renders a fault code the way it is named elsewhere: battery fault
// 35 is B35, motor fault 11 is E11.
//
// A code that already carries a letter is left alone, and a leading minus — a
// cleared fault in the events stream — keeps its sign.
func FaultLabel(series FaultSeries, code string) string {
	code = strings.TrimSpace(code)
	switch {
	case code == "":
		return ""
	case strings.HasPrefix(code, "-"):
		return "-" + string(series) + strings.TrimPrefix(code, "-")
	case code[0] < '0' || code[0] > '9':
		return code
	}
	return string(series) + code
}
