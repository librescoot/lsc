package doctor

import (
	"errors"
	"testing"
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
	if got.Detail != "battery 0: 35; battery 1: 35" {
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
