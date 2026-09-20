package boot

import (
	"strconv"
	"testing"
	"time"
)

func TestValidateBootTheme(t *testing.T) {
	for _, name := range []string{"librescoot", "windowsxp", "librescoot-xp", "coopertino", "a", "a.b_c-1"} {
		if err := validateBootTheme(name); err != nil {
			t.Errorf("validateBootTheme(%q) = %v, want nil", name, err)
		}
	}
	// Injection and shapes the DBC would have to defend against.
	for _, name := range []string{"", "-leading", ".dotted", "has space", "new\nline", "tab\tname", "slash/name", "semi;colon", "quo'te"} {
		if err := validateBootTheme(name); err == nil {
			t.Errorf("validateBootTheme(%q) = nil, want error", name)
		}
	}
}

func TestThemeFromState(t *testing.T) {
	if theme, ok := themeFromState(map[string]string{"theme": "coopertino"}); !ok || theme != "coopertino" {
		t.Fatalf("themeFromState = (%q, %v), want (coopertino, true)", theme, ok)
	}
	// Never reported: an asleep DBC looks exactly like this.
	if theme, ok := themeFromState(map[string]string{}); ok || theme != "" {
		t.Fatalf("themeFromState(empty) = (%q, %v), want (\"\", false)", theme, ok)
	}
	if _, ok := themeFromState(map[string]string{"theme": "   "}); ok {
		t.Fatal("themeFromState(whitespace) reported a value")
	}
}

func TestSoundFromState(t *testing.T) {
	cases := []struct {
		state    map[string]string
		want     bool
		reported bool
	}{
		{map[string]string{"sound": "1"}, true, true},
		{map[string]string{"sound": "0"}, false, true},
		{map[string]string{"sound": "off"}, false, true},
		{map[string]string{}, true, false},
		{map[string]string{"sound": ""}, true, false},
		{map[string]string{"sound": "maybe"}, true, false},
	}
	for _, tc := range cases {
		got, reported := soundFromState(tc.state)
		if got != tc.want || reported != tc.reported {
			t.Errorf("soundFromState(%v) = (%v, %v), want (%v, %v)",
				tc.state, got, reported, tc.want, tc.reported)
		}
	}
}

func TestParseSoundValue(t *testing.T) {
	for _, raw := range []string{"", "1", "on", "TRUE", "Yes"} {
		got, err := parseSoundValue(raw)
		if err != nil || !got {
			t.Errorf("parseSoundValue(%q) = (%v, %v), want (true, nil)", raw, got, err)
		}
	}
	for _, raw := range []string{"0", "off", "No", "FALSE"} {
		got, err := parseSoundValue(raw)
		if err != nil || got {
			t.Errorf("parseSoundValue(%q) = (%v, %v), want (false, nil)", raw, got, err)
		}
	}
	if _, err := parseSoundValue("maybe"); err == nil {
		t.Error("parseSoundValue(maybe) = nil error, want error")
	}
}

func TestResolveBootSound(t *testing.T) {
	cases := []struct {
		arg     string
		current bool
		want    bool
		wantErr bool
	}{
		{"on", false, true, false},
		{"off", true, false, false},
		{"toggle", true, false, false},
		{"toggle", false, true, false},
		{"ON", false, true, false},
		{"", true, true, true},
		{"sideways", true, true, true},
	}
	for _, tc := range cases {
		got, err := resolveBootSound(tc.arg, tc.current)
		if (err != nil) != tc.wantErr {
			t.Errorf("resolveBootSound(%q, %v) error = %v, wantErr %v", tc.arg, tc.current, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("resolveBootSound(%q, %v) = %v, want %v", tc.arg, tc.current, got, tc.want)
		}
	}
}

func TestNoteExplanation(t *testing.T) {
	for _, note := range []string{"unknown-theme", "bad-sound", "env-error"} {
		if got := noteExplanation(note); got == "" || got == note {
			t.Errorf("noteExplanation(%q) = %q, want a human sentence", note, got)
		}
	}
	if got := noteExplanation(""); got == "" {
		t.Error("noteExplanation(\"\") = empty, want a sentence")
	}
	// An unrecognised note is passed through rather than swallowed.
	if got := noteExplanation("something-new"); got != "something-new" {
		t.Errorf("noteExplanation(unknown) = %q, want it passed through", got)
	}
}

func TestDescribeAge(t *testing.T) {
	now := time.Now().Unix()
	if got := describeAge(map[string]string{"updated": ""}); got != "" {
		t.Errorf("describeAge(no timestamp) = %q, want empty", got)
	}
	if got := describeAge(map[string]string{"updated": "not-a-number"}); got != "" {
		t.Errorf("describeAge(garbage) = %q, want empty", got)
	}
	if got := describeAge(map[string]string{"updated": "0"}); got != "" {
		t.Errorf("describeAge(epoch 0) = %q, want empty", got)
	}
	if got := describeAge(map[string]string{"updated": itoa(now)}); got != "just now" {
		t.Errorf("describeAge(now) = %q, want \"just now\"", got)
	}
	if got := describeAge(map[string]string{"updated": itoa(now - 3*3600)}); got == "" {
		t.Error("describeAge(3h) = empty, want a duration")
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
