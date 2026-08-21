package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"librescoot/lsc/internal/redis"
)

func msg(id string, values map[string]interface{}) redis.XMessage {
	return redis.XMessage{ID: id, Values: values}
}

func TestParseFaultEventsEmpty(t *testing.T) {
	if got := parseFaultEvents(nil); len(got) != 0 {
		t.Fatalf("expected no events, got %d", len(got))
	}
}

// A scooter that raised no faults never creates the key at all, so XREVRANGE
// returns nothing. The bundle still has to contain the file.
func TestRenderFaultEventsEmpty(t *testing.T) {
	out := string(renderFaultEvents(nil))
	if !strings.HasPrefix(out, faultEventsHeader) {
		t.Fatalf("missing header, got %q", out)
	}
	if !strings.Contains(out, faultEventsEmptyNote) {
		t.Fatalf("missing empty note, got %q", out)
	}
	if out != faultEventsHeader+faultEventsEmptyNote {
		t.Fatalf("unexpected trailing content: %q", out)
	}
}

func TestWriteFaultEventsCreatesFileWhenStreamIsAbsent(t *testing.T) {
	dir := t.TempDir()

	n, err := writeFaultEvents(dir, parseFaultEvents(nil))
	if err != nil {
		t.Fatalf("writeFaultEvents: %v", err)
	}
	if n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}

	data, err := os.ReadFile(filepath.Join(dir, faultEventsFilename))
	if err != nil {
		t.Fatalf("expected the file to exist: %v", err)
	}
	if string(data) != faultEventsHeader+faultEventsEmptyNote {
		t.Fatalf("unexpected contents: %q", string(data))
	}
}

func TestParseFaultEventsReversesToOldestFirst(t *testing.T) {
	// XREVRANGE hands back newest first.
	events := parseFaultEvents([]redis.XMessage{
		msg("1761400500456-0", map[string]interface{}{"group": "vehicle", "code": "-3"}),
		msg("1761400443123-0", map[string]interface{}{"group": "vehicle", "code": "3", "description": "CAN bus timeout"}),
	})

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "1761400443123-0" || events[1].ID != "1761400500456-0" {
		t.Fatalf("not oldest-first: %q then %q", events[0].ID, events[1].ID)
	}
	if !events[0].Time.Before(events[1].Time) {
		t.Fatalf("timestamps not ascending: %v then %v", events[0].Time, events[1].Time)
	}
}

func TestParseFaultEventsDecodesIDTimestamp(t *testing.T) {
	events := parseFaultEvents([]redis.XMessage{
		msg("1761400443123-7", map[string]interface{}{"group": "vehicle", "code": "3"}),
	})

	want := time.UnixMilli(1761400443123)
	if !events[0].Time.Equal(want) {
		t.Fatalf("time = %v, want %v", events[0].Time, want)
	}

	line := string(renderFaultEvents(events))
	if !strings.Contains(line, want.UTC().Format("2006-01-02T15:04:05.000Z07:00")) {
		t.Fatalf("rendered time missing from %q", line)
	}
}

func TestParseFaultEventsMalformedID(t *testing.T) {
	events := parseFaultEvents([]redis.XMessage{
		msg("not-an-id", map[string]interface{}{"group": "vehicle", "code": "3"}),
	})

	if !events[0].Time.IsZero() {
		t.Fatalf("expected zero time for unparseable ID, got %v", events[0].Time)
	}
	if got := string(renderFaultEvents(events)); !strings.Contains(got, "not-an-id  -  RAISE") {
		t.Fatalf("expected dash for unknown time, got %q", got)
	}
}

func TestClassifyFaultCode(t *testing.T) {
	tests := []struct {
		raw      string
		wantKind string
		wantCode string
	}{
		{"3", faultKindRaise, "3"},
		{"-3", faultKindClear, "3"},
		{"0", faultKindRaise, "0"},
		// A clear of code 0 is written as "-0". Parsing before checking the
		// sign would collapse this into a raise.
		{"-0", faultKindClear, "0"},
		{" -12 ", faultKindClear, "12"},
		{"", faultKindUnknown, ""},
	}

	for _, tt := range tests {
		kind, code := classifyFaultCode(tt.raw)
		if kind != tt.wantKind || code != tt.wantCode {
			t.Errorf("classifyFaultCode(%q) = (%q, %q), want (%q, %q)",
				tt.raw, kind, code, tt.wantKind, tt.wantCode)
		}
	}
}

func TestRenderFaultEventsMarksClears(t *testing.T) {
	events := parseFaultEvents([]redis.XMessage{
		// Clears carry no description.
		msg("1761400500456-0", map[string]interface{}{"group": "battery:0", "code": "-5"}),
		msg("1761400443123-0", map[string]interface{}{"group": "battery:0", "code": "5", "description": "BMS over-temperature"}),
	})

	lines := strings.Split(strings.TrimSuffix(string(renderFaultEvents(events)), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 2 header lines and 2 entries, got %d lines: %q", len(lines), lines)
	}

	raise := lines[2]
	clear := lines[3]
	if !strings.Contains(raise, "RAISE  battery:0  5  BMS over-temperature") {
		t.Errorf("raise line = %q", raise)
	}
	if !strings.Contains(clear, "CLEAR  battery:0  5") {
		t.Errorf("clear line = %q", clear)
	}
	if strings.Contains(clear, "RAISE") {
		t.Errorf("clear line marked as raise: %q", clear)
	}
}

func TestFaultFieldFlattensWhitespace(t *testing.T) {
	events := parseFaultEvents([]redis.XMessage{
		msg("1761400443123-0", map[string]interface{}{
			"group":       "vehicle",
			"code":        "3",
			"description": "line one\nline two\ttabbed\r\n",
		}),
	})

	if got := events[0].Description; got != "line one line two tabbed" {
		t.Fatalf("description = %q", got)
	}
	if got := string(renderFaultEvents(events)); strings.Count(got, "\n") != 3 {
		t.Fatalf("expected 2 header lines plus 1 entry line, got %q", got)
	}
}

func TestFaultFieldMissingKey(t *testing.T) {
	events := parseFaultEvents([]redis.XMessage{
		msg("1761400443123-0", map[string]interface{}{"code": "3"}),
	})

	if events[0].Group != "" {
		t.Fatalf("expected empty group, got %q", events[0].Group)
	}
	if got := string(renderFaultEvents(events)); !strings.Contains(got, "RAISE  -  3") {
		t.Fatalf("expected dash for missing group, got %q", got)
	}
}
