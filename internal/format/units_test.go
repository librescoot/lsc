package format

import "testing"

// Bytes reports decimal SI units. The labels are load-bearing: a reader
// compares these against the sizes the tile repos publish and against what
// scootui-qt shows, both of which are decimal.
func TestBytesIsDecimal(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{1500, "1.5 kB"},
		{999999, "1000.0 kB"},
		{1000000, "1.0 MB"},
		// The routing tile archive that made the mislabelling visible: 192.7
		// when divided by 1024^2, which is what this must no longer print.
		{202055680, "202.1 MB"},
		{142409728, "142.4 MB"},
		{1000000000, "1.0 GB"},
		{2500000000, "2.5 GB"},
	}
	for _, tc := range tests {
		if got := Bytes(tc.n); got != tc.want {
			t.Errorf("Bytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// Guards the specific regression: a binary divisor behind a decimal label.
func TestBytesNotBinary(t *testing.T) {
	if got := Bytes(202055680); got == "192.7 MB" {
		t.Errorf("Bytes divided by 1024^2 while labelling the result MB: %q", got)
	}
	if got := Bytes(1048576); got != "1.0 MB" {
		t.Errorf("Bytes(1 MiB) = %q, want 1.0 MB (1 MiB is not 1 MB)", got)
	}
}

func TestMilliampHoursToAmpHours(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0", ""},         // non-positive reads as no data, callers skip the line
		{"-5", ""},        // negative likewise
		{"junk", ""},      // unparsable likewise
		{"", ""},          // missing field likewise
		{"999", "1.0 Ah"}, // rounds half up like the other formatters
		{"1000", "1.0 Ah"},
		{"17200", "17.2 Ah"},
		{"20000", "20.0 Ah"},
	}
	for _, tc := range tests {
		if got := MilliampHoursToAmpHours(tc.in); got != tc.want {
			t.Errorf("MilliampHoursToAmpHours(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The AUX tiers are the dashboard's (scootui-qt BatteryDisplay.qml), so the CLI
// cannot call a pack low while the display is happy. The regression this guards:
// lsc coloured 11.8V red while the dashboard still showed a healthy AUX.
func TestFormatAuxVoltageColoredMatchesDashboardTiers(t *testing.T) {
	EnableColors()
	defer DisableColors()

	tests := []struct {
		mv    string
		color string
	}{
		{"16026", colorGreen},  // 16.0V: high, not low - never red
		{"12400", colorGreen},  // the old "good" line is now well inside green
		{"11700", colorGreen},  // soft low line: at it is still fine
		{"11699", colorYellow}, // just below it
		{"11495", colorYellow}, // firmware empty line: warning, not yet orange
		{"11494", colorOrange}, // below it
		{"11000", colorOrange}, // critical line: at it is orange still
		{"10999", colorRed},    // below it
		{"0", colorGray},       // never reported
	}
	for _, tc := range tests {
		want := tc.color + MillivoltsToVolts(tc.mv) + colorReset
		if got := FormatAuxVoltageColored(tc.mv); got != want {
			t.Errorf("FormatAuxVoltageColored(%q) = %q, want %q", tc.mv, got, want)
		}
	}
}
