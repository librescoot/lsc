package ota

import (
	"strings"
	"testing"
)

func TestSetRunningVersionFields(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		component := make(map[string]any)
		setRunningVersionFields(component, "1.4.0", true)

		if got := component["running-version"]; got != "1.4.0" {
			t.Errorf("running-version = %v, want 1.4.0", got)
		}
		if got := component["installed-version"]; got != component["running-version"] {
			t.Errorf("installed-version alias = %v, want %v", got, component["running-version"])
		}
	})

	t.Run("unknown", func(t *testing.T) {
		component := make(map[string]any)
		setRunningVersionFields(component, "", false)

		if component["running-version"] != nil {
			t.Errorf("running-version = %v, want nil", component["running-version"])
		}
		if component["installed-version"] != nil {
			t.Errorf("installed-version = %v, want nil", component["installed-version"])
		}
	})
}

func TestComponentStatusSummaryAnnotatesCachedState(t *testing.T) {
	got := (componentStatus{Status: "pending-reboot", StateOrigin: "cached", UpdateVersion: "1.4.0"}).summary()
	if !strings.Contains(got, "cached") {
		t.Fatalf("summary = %q, want cached annotation", got)
	}
}

func TestComponentStatusSummaryLabelsUpdateVersion(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "downloading target", status: "downloading", want: "target 1.4.0"},
		{name: "preparing target", status: "preparing", want: "target 1.4.0"},
		{name: "installing target", status: "installing", want: "target 1.4.0"},
		{name: "pending reboot", status: "pending-reboot", want: "pending 1.4.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (componentStatus{Status: tt.status, UpdateVersion: "1.4.0"}).summary()
			if !strings.Contains(got, tt.want) {
				t.Errorf("summary = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
