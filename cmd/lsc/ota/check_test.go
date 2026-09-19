package ota

import (
	"strings"
	"testing"
	"time"
)

func TestEffectiveCheckTimeout(t *testing.T) {
	t.Cleanup(func() { checkTimeout = 0 })

	tests := []struct {
		name           string
		override       time.Duration
		targets        []string
		orchestrateDBC bool
		want           time.Duration
	}{
		{name: "mdb uses the short default", targets: []string{"mdb"}, want: checkDefaultTimeout},
		{name: "orchestrated dbc waits for the boot", targets: []string{"dbc"}, orchestrateDBC: true, want: checkDBCOrchestratedTimeout},
		{name: "unorchestrated dbc must already be on", targets: []string{"dbc"}, want: checkDefaultTimeout},
		{name: "both boards take the slowest", targets: []string{"mdb", "dbc"}, orchestrateDBC: true, want: checkDBCOrchestratedTimeout},
		{name: "orchestration is ignored without dbc", targets: []string{"mdb"}, orchestrateDBC: true, want: checkDefaultTimeout},
		{name: "explicit timeout wins", override: 3 * time.Second, targets: []string{"mdb", "dbc"}, orchestrateDBC: true, want: 3 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkTimeout = tt.override
			if got := effectiveCheckTimeout(tt.targets, tt.orchestrateDBC); got != tt.want {
				t.Errorf("effectiveCheckTimeout(%v, %v) = %s, want %s", tt.targets, tt.orchestrateDBC, got, tt.want)
			}
		})
	}
}

func TestDBCCheckWillBeOrchestrated(t *testing.T) {
	tests := []struct {
		name      string
		targets   []string
		settingOn bool
		want      bool
	}{
		{name: "both boards let the MDB power the DBC on", targets: []string{"mdb", "dbc"}, settingOn: true, want: true},
		{name: "dbc only is not chased by the MDB", targets: []string{"dbc"}, settingOn: true, want: false},
		{name: "orchestration off never powers it on", targets: []string{"mdb", "dbc"}, settingOn: false, want: false},
		{name: "mdb only needs no dbc reasoning", targets: []string{"mdb"}, settingOn: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dbcCheckWillBeOrchestrated(tt.targets, tt.settingOn); got != tt.want {
				t.Errorf("dbcCheckWillBeOrchestrated(%v, %v) = %v, want %v", tt.targets, tt.settingOn, got, tt.want)
			}
		})
	}
}

func TestWaitNoticeExplainsTheDBCWait(t *testing.T) {
	if got := waitNotice([]string{"dbc"}, checkDBCOrchestratedTimeout, true); !strings.Contains(got, "orchestration") {
		t.Errorf("orchestrated wait notice should name orchestration, got %q", got)
	}
	if got := waitNotice([]string{"dbc"}, checkDefaultTimeout, false); !strings.Contains(got, "already be on") {
		t.Errorf("unorchestrated wait notice should say the DBC must be on, got %q", got)
	}
	if got := waitNotice([]string{"mdb"}, checkDefaultTimeout, false); strings.Contains(got, "DBC") {
		t.Errorf("mdb wait notice should not mention the DBC, got %q", got)
	}
}

func TestPendingMessageDefersTheDBC(t *testing.T) {
	deferred := checkOutcome{component: "dbc", kind: "pending"}.pendingMessage()
	if !strings.Contains(deferred, "next time it is on") {
		t.Errorf("a DBC nothing powered on should defer the queued check, got %q", deferred)
	}

	orchestrated := checkOutcome{component: "dbc", kind: "pending", orchestrated: true}.pendingMessage()
	if !strings.Contains(orchestrated, "ota watch") {
		t.Errorf("an orchestrated DBC is still booting, so it should point at watch, got %q", orchestrated)
	}

	mdb := checkOutcome{component: "mdb", kind: "pending"}.pendingMessage()
	if !strings.Contains(mdb, "ota watch") {
		t.Errorf("MDB should keep the generic pending message, got %q", mdb)
	}
}

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
