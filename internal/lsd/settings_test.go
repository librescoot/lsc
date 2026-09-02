package lsd

import (
	"testing"

	"librescoot/lsc/internal/schema"
)

func TestValidateSettingWithSchema(t *testing.T) {
	sch, err := schema.Parse([]byte(`{
		"alarm.duration": {"type":"int","min":0,"max":300},
		"pm.default-state": {"type":"enum","values":[{"value":"run"},{"value":"suspend"}]},
		"alarm.enabled": {"type":"bool"},
		"updates.url": {"type":"url"},
		"pm.delay": {"type":"duration"},
		"system.serial": {"type":"string","read-only":true}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{schema: sch}
	tests := []struct {
		key, value string
		ok         bool
	}{
		{"alarm.duration", "30", true},
		{"alarm.duration", "301", false},
		{"alarm.duration", "abc", false},
		{"alarm.duration", "", true},
		{"pm.default-state", "run", true},
		{"pm.default-state", "hover", false},
		{"alarm.enabled", "true", true},
		{"alarm.enabled", "yes", false},
		{"updates.url", "https://x", true},
		{"updates.url", "x", false},
		{"pm.delay", "5m", true},
		{"pm.delay", "5 minutes", false},
		{"system.serial", "x", false},
		{"nope.unknown", "x", false},
	}
	for _, tt := range tests {
		err := s.validateSetting(tt.key, tt.value)
		if (err == nil) != tt.ok {
			t.Errorf("validateSetting(%q, %q) err=%v, want ok=%v", tt.key, tt.value, err, tt.ok)
		}
	}
}
