package ota

import "testing"

func TestPlanKind(t *testing.T) {
	tests := []struct {
		status string
		kind   string
		ok     bool
	}{
		{status: "downloading", kind: "update", ok: true},
		{status: "preparing", kind: "update", ok: true},
		{status: "installing", kind: "update", ok: true},
		{status: "pending-reboot", kind: "update", ok: true},
		{status: "error", kind: "error", ok: true},
		{status: "idle"},
		{status: ""},
		{status: "staged-noop"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			kind, ok := planKind(tt.status)
			if kind != tt.kind || ok != tt.ok {
				t.Errorf("planKind(%q) = (%q, %v), want (%q, %v)", tt.status, kind, ok, tt.kind, tt.ok)
			}
		})
	}
}

func TestCheckOutcomeJSON(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		got := checkOutcome{component: "mdb", kind: "update", status: componentStatus{
			Status:         "downloading",
			UpdateVersion:  "1.4.0",
			UpdateMethod:   "delta",
			RunningVersion: "nightly-old",
		}}.json()

		if got["component"] != "mdb" || got["outcome"] != "update-available" {
			t.Fatalf("unexpected component/outcome: %v", got)
		}
		if got["update-version"] != "1.4.0" || got["update-method"] != "delta" {
			t.Fatalf("missing plan fields: %v", got)
		}
		if got["running-version"] != "nightly-old" {
			t.Fatalf("missing running version: %v", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		got := checkOutcome{component: "dbc", kind: "error", status: componentStatus{
			Status:       "error",
			Error:        "download-failed",
			ErrorMessage: "network unreachable",
		}}.json()

		if got["outcome"] != "error" || got["error"] != "download-failed" || got["error-message"] != "network unreachable" {
			t.Fatalf("unexpected error outcome: %v", got)
		}
	})

	t.Run("no update", func(t *testing.T) {
		got := checkOutcome{component: "mdb", kind: "no-update", status: componentStatus{
			Status:         "idle",
			RunningVersion: "1.4.0",
		}}.json()

		if got["outcome"] != "no-update" || got["running-version"] != "1.4.0" {
			t.Fatalf("unexpected no-update outcome: %v", got)
		}
		if _, ok := got["update-version"]; ok {
			t.Fatalf("no-update outcome should not carry a target: %v", got)
		}
	})

	t.Run("pending", func(t *testing.T) {
		got := checkOutcome{component: "dbc", kind: "pending", status: componentStatus{Status: "idle"}}.json()

		if got["outcome"] != "pending" {
			t.Fatalf("unexpected pending outcome: %v", got)
		}
	})
}
