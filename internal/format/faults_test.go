package format

import "testing"

func TestFaultLabel(t *testing.T) {
	tests := []struct {
		series FaultSeries
		code   string
		want   string
	}{
		{SeriesBattery, "35", "B35"},
		{SeriesMotor, "11", "E11"},
		{SeriesVehicle, "3", "V3"},
		// Already labelled: do not stack a second letter on it.
		{SeriesBattery, "B35", "B35"},
		{SeriesMotor, "E11", "E11"},
		// A cleared fault in the events stream keeps its sign.
		{SeriesBattery, "-35", "-B35"},
		{SeriesBattery, " 35 ", "B35"},
		{SeriesBattery, "", ""},
	}
	for _, tt := range tests {
		if got := FaultLabel(tt.series, tt.code); got != tt.want {
			t.Errorf("FaultLabel(%q, %q) = %q, want %q", tt.series, tt.code, got, tt.want)
		}
	}
}
