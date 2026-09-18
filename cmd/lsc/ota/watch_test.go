package ota

import "testing"

func TestWatchCompletionFor(t *testing.T) {
	tests := []struct {
		name string
		prev componentStatus
		now  componentStatus
		want bool
	}{
		{
			name: "pending reboot resolves to idle",
			prev: componentStatus{Status: "pending-reboot", UpdateVersion: "1.4.0"},
			now:  componentStatus{Status: "idle", RunningVersion: "1.4.0"},
			want: true,
		},
		{
			name: "pending reboot resolves to empty status",
			prev: componentStatus{Status: "pending-reboot", UpdateVersion: "1.4.0"},
			now:  componentStatus{Status: "", RunningVersion: "1.4.0"},
			want: true,
		},
		{
			name: "idle to idle is not a completion",
			prev: componentStatus{Status: "idle", UpdateVersion: "1.4.0"},
			now:  componentStatus{Status: "idle", RunningVersion: "1.4.0"},
		},
		{
			name: "pending reboot handing over to a new download is not a completion",
			prev: componentStatus{Status: "pending-reboot", UpdateVersion: "1.4.0"},
			now:  componentStatus{Status: "downloading", UpdateVersion: "1.5.0"},
		},
		{
			name: "pending reboot to error is not a completion",
			prev: componentStatus{Status: "pending-reboot", UpdateVersion: "1.4.0"},
			now:  componentStatus{Status: "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watchCompletionFor(tt.prev, tt.now)
			if (got != nil) != tt.want {
				t.Fatalf("watchCompletionFor() = %v, want non-nil %v", got, tt.want)
			}
			if tt.want && got.targetVersion != tt.prev.UpdateVersion {
				t.Errorf("targetVersion = %q, want %q", got.targetVersion, tt.prev.UpdateVersion)
			}
		})
	}
}

func TestWatchCompletionSummary(t *testing.T) {
	tests := []struct {
		name       string
		completion watchCompletion
		want       string
	}{
		{
			name:       "running matches target",
			completion: watchCompletion{targetVersion: "1.4.0", runningVersion: "1.4.0"},
			want:       "update complete — running 1.4.0",
		},
		{
			name:       "version not yet visible",
			completion: watchCompletion{targetVersion: "1.4.0", runningVersion: "1.3.9"},
			want:       "update complete — target 1.4.0",
		},
		{
			name:       "no target recorded",
			completion: watchCompletion{runningVersion: "1.4.0"},
			want:       "update complete — running 1.4.0",
		},
		{
			name:       "no versions at all",
			completion: watchCompletion{},
			want:       "update complete — rebooted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.completion.summary(); got != tt.want {
				t.Errorf("summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
