package lsc

import (
	"reflect"
	"testing"

	"librescoot/lsc/internal/schema"

	"github.com/spf13/cobra"
)

func TestSettingsSetCompletionCandidatesCompletesKeys(t *testing.T) {
	s := &schema.Schema{Settings: map[string]schema.Setting{
		"alarm.enabled": {Type: "bool", Description: "Enable the alarm"},
		"alarm.honk":    {Type: "bool", Description: "Sound the horn"},
		"alarm.state":   {Type: "bool", ReadOnly: true},
		"updates.channel": {
			Type: "enum",
		},
	}}

	got := settingsSetCompletionCandidates(s, nil, []string{"alarm.enabled", "true"}, "alarm.")
	want := []string{"alarm.honk\tSound the horn"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("completion candidates = %#v, want %#v", got, want)
	}
}

func TestSettingsKeyCompletionCandidatesIncludesRedisAndIndexedKeys(t *testing.T) {
	s := &schema.Schema{Settings: map[string]schema.Setting{
		"dashboard.saved-locations.0.label": {
			Type:        "string",
			Pattern:     "indexed",
			Description: "Saved location label",
		},
		"system.state": {Type: "string", ReadOnly: true},
	}}
	settings := map[string]string{
		"dashboard.saved-locations.3.label": "Home",
		"custom.setting":                    "value",
		"system.state":                      "ready",
	}

	got := settingsKeyCompletionCandidates(s, settings, nil, "", true, false)
	want := []string{
		"custom.setting",
		"dashboard.saved-locations.0.label\tSaved location label",
		"dashboard.saved-locations.3.label\tSaved location label",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("completion candidates = %#v, want %#v", got, want)
	}
}

func TestSettingsSetCompletionCandidatesCompletesValues(t *testing.T) {
	s := &schema.Schema{Settings: map[string]schema.Setting{
		"alarm.enabled": {Type: "bool"},
		"updates.channel": {
			Type: "enum",
			Values: []schema.EnumValue{
				{Value: "stable", Label: "Stable releases"},
				{Value: "testing", Label: "Testing releases"},
			},
		},
	}}

	tests := []struct {
		name       string
		args       []string
		toComplete string
		want       []string
	}{
		{
			name:       "boolean",
			args:       []string{"alarm.enabled"},
			toComplete: "t",
			want:       []string{"true"},
		},
		{
			name:       "enum",
			args:       []string{"updates.channel"},
			toComplete: "s",
			want:       []string{"stable\tStable releases"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settingsSetCompletionCandidates(s, nil, tt.args, tt.toComplete)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("completion candidates = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSetCommandsHaveArgumentCompletion(t *testing.T) {
	if settingsSetCmd.ValidArgsFunction == nil {
		t.Fatal("settings set command has no argument completion")
	}
	if setCmd.ValidArgsFunction == nil {
		t.Fatal("set shortcut has no argument completion")
	}
	for name, cmd := range map[string]*cobra.Command{
		"settings get": settingsGetCmd,
		"get":          getCmd,
		"settings del": settingsDelCmd,
		"del":          delCmd,
	} {
		if cmd.ValidArgsFunction == nil {
			t.Fatalf("%s command has no argument completion", name)
		}
	}
}
