package doctor

import (
	"errors"
	"testing"
	"time"
)

// fakeState serves the hashes and sets a check reads.
type fakeState struct {
	hashes map[string]map[string]string
	sets   map[string][]string
	err    error
}

func (f fakeState) HGetAll(key string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if hash, ok := f.hashes[key]; ok {
		return hash, nil
	}
	return map[string]string{}, nil
}

func (f fakeState) SMembers(key string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sets[key], nil
}

func healthyEnv() env {
	return env{
		state: fakeState{hashes: map[string]map[string]string{
			"version:mdb": {"id": "librescoot-mdb", "kernel_check": "ok", "kernel_release": "6.12.34-mdb"},
			"version:dbc": {"id": "librescoot-dbc", "kernel_check": "ok", "kernel_release": "6.12.34-dbc"},
			"ota":         {"status:mdb": "idle", "status:dbc": "idle"},
		}},
		failedUnits: func() []string { return nil },
		freeBytes:   func(string) (uint64, uint64, error) { return 2 << 30, 6 << 30, nil },
		link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkUSB}, nil },
		now:         func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func TestCheckKernel(t *testing.T) {
	tests := []struct {
		name        string
		hashes      map[string]map[string]string
		wantVerdict Verdict
		wantDetail  string
	}{
		{
			name: "both boards agree",
			hashes: map[string]map[string]string{
				"version:mdb": {"id": "librescoot-mdb", "kernel_check": "ok"},
				"version:dbc": {"id": "librescoot-dbc", "kernel_check": "ok"},
			},
			wantVerdict: VerdictOK,
		},
		{
			name: "a board reports skew",
			hashes: map[string]map[string]string{
				"version:mdb": {"id": "librescoot-mdb", "kernel_check": "ok"},
				"version:dbc": {
					"id":                  "librescoot-dbc",
					"kernel_check":        "skew",
					"kernel_check_detail": "no /lib/modules/6.12.34-dbc",
				},
			},
			wantVerdict: VerdictAttention,
			wantDetail:  "dbc: no /lib/modules/6.12.34-dbc",
		},
		{
			name: "an older writer does not report it",
			hashes: map[string]map[string]string{
				"version:mdb": {"id": "librescoot-mdb", "version_id": "nightly-x"},
			},
			wantVerdict: VerdictUnknown,
		},
		{
			name:        "no board reports anything",
			hashes:      map[string]map[string]string{},
			wantVerdict: VerdictUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := healthyEnv()
			e.state = fakeState{hashes: tt.hashes}
			result := checkKernel(e)
			if result.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %v (%s), want %v", result.Verdict, result.Detail, tt.wantVerdict)
			}
			if tt.wantDetail != "" && result.Detail != tt.wantDetail {
				t.Fatalf("detail = %q, want %q", result.Detail, tt.wantDetail)
			}
		})
	}
}

func TestCheckLink(t *testing.T) {
	tests := []struct {
		name        string
		link        func() (Link, error)
		wantVerdict Verdict
		wantDetail  string
	}{
		{
			name:        "USB link",
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkUSB}, nil },
			wantVerdict: VerdictOK,
			wantDetail:  "DBC over the USB link",
		},
		{
			name:        "backup link",
			link:        func() (Link, error) { return Link{Peer: "MDB", Path: LinkBackup}, nil },
			wantVerdict: VerdictWarning,
			wantDetail:  "MDB over the PPP backup link",
		},
		{
			name:        "no route",
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkDown}, nil },
			wantVerdict: VerdictAttention,
			wantDetail:  "no route to the DBC",
		},
		{
			name:        "not on a board",
			link:        func() (Link, error) { return Link{}, nil },
			wantVerdict: VerdictUnknown,
		},
		{
			name:        "no routing table",
			link:        nil,
			wantVerdict: VerdictUnknown,
		},
		{
			name:        "ip failed",
			link:        func() (Link, error) { return Link{}, errors.New("ip: not found") },
			wantVerdict: VerdictUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := healthyEnv()
			e.link = tt.link
			got := checkLink(e)
			if got.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %v (%s), want %v", got.Verdict, got.Detail, tt.wantVerdict)
			}
			if tt.wantDetail != "" && got.Detail != tt.wantDetail {
				t.Fatalf("detail = %q, want %q", got.Detail, tt.wantDetail)
			}
		})
	}
}

// The last powered session is what a support run reads: once the laptop holds
// the MDB's USB port, the live route says nothing about normal operation.
func TestCheckLinkUsesLastLock(t *testing.T) {
	const now = 1_700_000_000
	tests := []struct {
		name        string
		system      map[string]string
		link        func() (Link, error)
		wantVerdict Verdict
		wantDetail  string
	}{
		{
			name:        "last lock on USB, DBC off now",
			system:      map[string]string{"dbc-link": "usb0", "dbc-link-at": "1699998200"},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkDown}, nil },
			wantVerdict: VerdictOK,
			wantDetail:  "DBC over the USB link at last lock, 30m ago; no route now",
		},
		{
			name:        "last lock on USB, backup in use during a support session",
			system:      map[string]string{"dbc-link": "usb0", "dbc-link-at": "1699998200"},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkBackup}, nil },
			wantVerdict: VerdictOK,
			wantDetail:  "DBC over the USB link at last lock, 30m ago; PPP now",
		},
		{
			name:        "last lock on the backup, USB has returned",
			system:      map[string]string{"dbc-link": "ppp0", "dbc-link-at": "1699998200"},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkUSB}, nil },
			wantVerdict: VerdictWarning,
			wantDetail:  "DBC over the PPP backup link at last lock, 30m ago; USB now",
		},
		{
			name:        "last lock unreachable",
			system:      map[string]string{"dbc-link": "none", "dbc-link-at": "1699998200"},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkUSB}, nil },
			wantVerdict: VerdictAttention,
			wantDetail:  "no DBC link at last lock, 30m ago; USB now",
		},
		{
			name:        "record without a timestamp",
			system:      map[string]string{"dbc-link": "usb0"},
			link:        func() (Link, error) { return Link{}, nil },
			wantVerdict: VerdictOK,
			wantDetail:  "DBC over the USB link at last lock",
		},
		{
			name:        "unrecognized transport",
			system:      map[string]string{"dbc-link": "wlan0", "dbc-link-at": "1699998200"},
			link:        func() (Link, error) { return Link{}, nil },
			wantVerdict: VerdictUnknown,
			wantDetail:  "DBC link unclear at last lock (wlan0), 30m ago",
		},
		{
			name: "USB carried traffic, backup was up",
			system: map[string]string{
				"dbc-link": "usb0", "dbc-usb": "up", "dbc-ppp": "up", "dbc-link-at": "1699998200",
			},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkDown}, nil },
			wantVerdict: VerdictOK,
			wantDetail:  "DBC over the USB link at last lock (USB up, PPP up), 30m ago; no route now",
		},
		{
			name: "USB carried traffic, backup never came up",
			system: map[string]string{
				"dbc-link": "usb0", "dbc-usb": "up", "dbc-ppp": "down", "dbc-link-at": "1699998200",
			},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkDown}, nil },
			wantVerdict: VerdictWarning,
			wantDetail:  "DBC over the USB link at last lock (USB up, PPP down), 30m ago; no route now",
		},
		{
			name: "ran on the backup",
			system: map[string]string{
				"dbc-link": "ppp0", "dbc-usb": "down", "dbc-ppp": "up", "dbc-link-at": "1699998200",
			},
			link:        func() (Link, error) { return Link{Peer: "DBC", Path: LinkDown}, nil },
			wantVerdict: VerdictWarning,
			wantDetail:  "DBC over the PPP backup link at last lock (USB down, PPP up), 30m ago; no route now",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := healthyEnv()
			e.now = func() time.Time { return time.Unix(now, 0) }
			e.state = fakeState{hashes: map[string]map[string]string{"system": tt.system}}
			e.link = tt.link
			got := checkLink(e)
			if got.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %v (%s), want %v", got.Verdict, got.Detail, tt.wantVerdict)
			}
			if got.Detail != tt.wantDetail {
				t.Fatalf("detail = %q, want %q", got.Detail, tt.wantDetail)
			}
		})
	}
}

func TestCheckServices(t *testing.T) {
	e := healthyEnv()
	if got := checkServices(e); got.Verdict != VerdictOK {
		t.Fatalf("healthy = %v (%s)", got.Verdict, got.Detail)
	}
	e.failedUnits = func() []string { return []string{"librescoot-vehicle.service"} }
	got := checkServices(e)
	if got.Verdict != VerdictAttention {
		t.Fatalf("failed unit = %v", got.Verdict)
	}
	if got.Detail != "librescoot-vehicle.service failed" {
		t.Fatalf("detail = %q", got.Detail)
	}
	e.failedUnits = nil
	if got := checkServices(e); got.Verdict != VerdictUnknown {
		t.Fatalf("no systemd = %v", got.Verdict)
	}
}

func TestCheckOTA(t *testing.T) {
	tests := []struct {
		name        string
		ota         map[string]string
		wantVerdict Verdict
	}{
		{name: "idle", ota: map[string]string{"status:mdb": "idle"}, wantVerdict: VerdictOK},
		{name: "waiting for reboot", ota: map[string]string{"status:mdb": "pending-reboot"}, wantVerdict: VerdictWarning},
		{name: "installing", ota: map[string]string{"status:dbc": "installing"}, wantVerdict: VerdictWarning},
		{
			name:        "failed with a message",
			ota:         map[string]string{"error-message:dbc": "fit check failed"},
			wantVerdict: VerdictAttention,
		},
		{
			name:        "failed status",
			ota:         map[string]string{"status:mdb": "install-failed"},
			wantVerdict: VerdictAttention,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := healthyEnv()
			e.state = fakeState{hashes: map[string]map[string]string{"ota": tt.ota}}
			if got := checkOTA(e); got.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %v (%s), want %v", got.Verdict, got.Detail, tt.wantVerdict)
			}
		})
	}
}

func TestCheckFaults(t *testing.T) {
	e := healthyEnv()
	if got := checkFaults(e); got.Verdict != VerdictOK {
		t.Fatalf("no faults = %v (%s)", got.Verdict, got.Detail)
	}
	e.state = fakeState{sets: map[string][]string{
		"battery:0:fault": {"35"},
		"battery:1:fault": {"35"},
	}}
	got := checkFaults(e)
	if got.Verdict != VerdictAttention {
		t.Fatalf("faults = %v", got.Verdict)
	}
	if got.Detail != "battery 0: B35; battery 1: B35" {
		t.Fatalf("detail = %q", got.Detail)
	}
	e.state = fakeState{err: errors.New("redis down")}
	if got := checkFaults(e); got.Verdict != VerdictUnknown {
		t.Fatalf("unreadable = %v", got.Verdict)
	}
}

func TestCheckStorageOnlyJudgesData(t *testing.T) {
	e := healthyEnv()
	if got := checkStorage(e); got.Verdict != VerdictOK {
		t.Fatalf("roomy = %v (%s)", got.Verdict, got.Detail)
	}
	e.freeBytes = func(path string) (uint64, uint64, error) {
		if path != "/data" {
			t.Fatalf("checked %s, want /data: the rootfs is a fixed-size slot", path)
		}
		return 40 << 20, 6 << 30, nil
	}
	if got := checkStorage(e); got.Verdict != VerdictAttention {
		t.Fatalf("nearly full = %v", got.Verdict)
	}
	e.freeBytes = func(string) (uint64, uint64, error) { return 300 << 20, 6 << 30, nil }
	if got := checkStorage(e); got.Verdict != VerdictWarning {
		t.Fatalf("low = %v", got.Verdict)
	}
}

// The report is only useful if a healthy scooter produces none.
func TestHealthyScooterIsAllOK(t *testing.T) {
	e := healthyEnv()
	for _, c := range allChecks() {
		if got := c.run(e); got.Verdict != VerdictOK {
			t.Errorf("%s = %v (%s)", c.name, got.Verdict, got.Detail)
		}
	}
}
