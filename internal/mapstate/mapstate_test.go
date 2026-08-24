package mapstate

import (
	"testing"
	"time"
)

func fullHash() map[string]string {
	return map[string]string{
		"region":               "berlin_brandenburg",
		"region-name":          "Berlin & Brandenburg",
		"map:sha256":           "78d1f829d3a1b4c5e6f708192a3b4c5d6e7f80912a3b4c5d6e7f80912a3b4c5d",
		"map:size":             "208076800",
		"map:published-at":     "2026-08-12T16:26:27Z",
		"map:mtime":            "2026-08-13T09:12:00Z",
		"routing:sha256":       "02f5fd5b1c2d3e4f5061728394a5b6c7d8e9f0a1b2c3d4e5f60718293a4b5c6d",
		"routing:size":         "202055680",
		"routing:published-at": "2026-08-09T21:30:13Z",
		"routing:mtime":        "2026-08-13T09:20:00Z",
		"last-update-check":    "2026-08-20T07:00:00Z",
		"update-available":     "false",
		"updated-at":           "2026-08-24T11:00:00Z",
	}
}

func TestParseFullHash(t *testing.T) {
	s := Parse(fullHash())

	if !s.Recorded {
		t.Error("Recorded = false, want true")
	}
	if s.Region != "berlin_brandenburg" {
		t.Errorf("Region = %q, want berlin_brandenburg", s.Region)
	}
	if s.RegionName != "Berlin & Brandenburg" {
		t.Errorf("RegionName = %q, want Berlin & Brandenburg", s.RegionName)
	}
	if !s.Map.Installed || !s.Routing.Installed {
		t.Errorf("Installed = %v/%v, want both true", s.Map.Installed, s.Routing.Installed)
	}
	if !s.Map.SizeKnown || s.Map.Size != 208076800 {
		t.Errorf("Map.Size = %d (known %v), want 208076800", s.Map.Size, s.Map.SizeKnown)
	}
	if s.Routing.Size != 202055680 {
		t.Errorf("Routing.Size = %d, want 202055680", s.Routing.Size)
	}
	if !s.Map.HasProvenance() || !s.Routing.HasProvenance() {
		t.Error("HasProvenance = false, want true for both")
	}
	if !s.UpdateAvailableKnown || s.UpdateAvailable {
		t.Errorf("UpdateAvailable = %v (known %v), want false/true", s.UpdateAvailable, s.UpdateAvailableKnown)
	}
	if s.LastUpdateCheck != "2026-08-20T07:00:00Z" || s.UpdatedAt != "2026-08-24T11:00:00Z" {
		t.Errorf("timestamps = %q / %q", s.LastUpdateCheck, s.UpdatedAt)
	}
}

// An absent hash is the normal state after an MDB reboot, and it must not be
// reported as "no tiles installed".
func TestParseEmptyHash(t *testing.T) {
	s := Parse(map[string]string{})

	if s.Recorded {
		t.Error("Recorded = true, want false for an absent hash")
	}
	if s.AnyInstalled() {
		t.Error("AnyInstalled = true, want false")
	}
	if s.UpdateAvailableKnown {
		t.Error("UpdateAvailableKnown = true, want false")
	}
	if s.RegionLabel() != "" {
		t.Errorf("RegionLabel = %q, want empty", s.RegionLabel())
	}
}

// Tiles flashed or copied in by hand have size and mtime but no digest.
func TestParseInstalledWithoutProvenance(t *testing.T) {
	s := Parse(map[string]string{
		"map:size":  "208076800",
		"map:mtime": "2026-08-13T09:12:00Z",
	})

	if !s.Recorded {
		t.Error("Recorded = false, want true")
	}
	if !s.Map.Installed {
		t.Error("Map.Installed = false, want true")
	}
	if s.Map.HasProvenance() {
		t.Error("Map.HasProvenance = true, want false")
	}
	if s.Routing.Installed {
		t.Error("Routing.Installed = true, want false")
	}
	if !s.AnyInstalled() {
		t.Error("AnyInstalled = false, want true")
	}
	if got, want := s.Map.Summary(), "198.4 MB, written 2026-08-13, provenance unknown"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
}

// The size field is the presence test. Leftover provenance without it does not
// make an artifact installed.
func TestParseSizeIsThePresenceTest(t *testing.T) {
	s := Parse(map[string]string{
		"routing:sha256":       "02f5fd5b1c2d",
		"routing:published-at": "2026-08-09T21:30:13Z",
	})

	if s.Routing.Installed {
		t.Error("Routing.Installed = true, want false without routing:size")
	}
	if got, want := s.Routing.Summary(), "not installed"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
	if got := s.Routing.JSON()["installed"]; got != false {
		t.Errorf("JSON installed = %v, want false", got)
	}
}

func TestParseUnparseableSize(t *testing.T) {
	s := Parse(map[string]string{"map:size": "lots"})

	if !s.Map.Installed {
		t.Error("Map.Installed = false, want true: the field is present")
	}
	if s.Map.SizeKnown {
		t.Error("Map.SizeKnown = true, want false")
	}
	if got, want := s.Map.Summary(), "installed, provenance unknown"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
	if got := s.Map.JSON()["size_bytes"]; got != nil {
		t.Errorf("JSON size_bytes = %v, want nil", got)
	}
}

func TestParseUpdateAvailable(t *testing.T) {
	tests := []struct {
		in         string
		wantKnown  bool
		wantUpdate bool
	}{
		{"true", true, true},
		{"false", true, false},
		{"", false, false},
		{"maybe", false, false},
	}

	for _, tt := range tests {
		s := Parse(map[string]string{"update-available": tt.in, "region": "x"})
		if s.UpdateAvailableKnown != tt.wantKnown || s.UpdateAvailable != tt.wantUpdate {
			t.Errorf("update-available %q -> %v (known %v), want %v (known %v)",
				tt.in, s.UpdateAvailable, s.UpdateAvailableKnown, tt.wantUpdate, tt.wantKnown)
		}
	}
}

func TestRegionLabel(t *testing.T) {
	tests := []struct {
		slug string
		name string
		want string
	}{
		{"berlin_brandenburg", "Berlin & Brandenburg", "Berlin & Brandenburg (berlin_brandenburg)"},
		{"berlin_brandenburg", "", "berlin_brandenburg"},
		{"", "Berlin & Brandenburg", "Berlin & Brandenburg"},
		{"bayern", "bayern", "bayern"},
		{"", "", ""},
	}

	for _, tt := range tests {
		s := State{Region: tt.slug, RegionName: tt.name}
		if got := s.RegionLabel(); got != tt.want {
			t.Errorf("RegionLabel(%q, %q) = %q, want %q", tt.slug, tt.name, got, tt.want)
		}
	}
}

func TestArtifactSummary(t *testing.T) {
	tests := []struct {
		name string
		a    Artifact
		want string
	}{
		{
			name: "full",
			a: Artifact{
				Installed: true, Size: 208076800, SizeKnown: true,
				SHA256:      "78d1f829d3a1b4c5e6f708192a3b4c5d",
				PublishedAt: "2026-08-12T16:26:27Z",
				MTime:       "2026-08-13T09:12:00Z",
			},
			want: "198.4 MB, 78d1f829d3a1, published 2026-08-12",
		},
		{
			name: "digest but no release",
			a: Artifact{
				Installed: true, Size: 1024, SizeKnown: true,
				SHA256: "78d1f829d3a1b4c5e6f708192a3b4c5d",
				MTime:  "2026-08-13T09:12:00Z",
			},
			want: "1.0 KB, 78d1f829d3a1, written 2026-08-13",
		},
		{
			name: "bare file",
			a:    Artifact{Installed: true, Size: 512, SizeKnown: true},
			want: "512 B, provenance unknown",
		},
		{
			name: "absent",
			a:    Artifact{},
			want: "not installed",
		},
	}

	for _, tt := range tests {
		if got := tt.a.Summary(); got != tt.want {
			t.Errorf("%s: Summary = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStateJSONNilsAbsentFields(t *testing.T) {
	out := Parse(map[string]string{"map:size": "10"}).JSON()

	for _, key := range []string{"region", "region_name", "last_update_check", "update_available", "updated_at"} {
		if out[key] != nil {
			t.Errorf("JSON %s = %v, want nil", key, out[key])
		}
	}
	if out["recorded"] != true {
		t.Errorf("JSON recorded = %v, want true", out["recorded"])
	}

	artifact, ok := out["map"].(map[string]any)
	if !ok {
		t.Fatalf("JSON map = %T, want map[string]any", out["map"])
	}
	if artifact["installed"] != true {
		t.Errorf("JSON map.installed = %v, want true", artifact["installed"])
	}
	if artifact["size_bytes"] != int64(10) {
		t.Errorf("JSON map.size_bytes = %v, want 10", artifact["size_bytes"])
	}
	for _, key := range []string{"sha256", "published_at", "mtime"} {
		if artifact[key] != nil {
			t.Errorf("JSON map.%s = %v, want nil", key, artifact[key])
		}
	}
}

func TestShortDigest(t *testing.T) {
	tests := []struct{ in, want string }{
		{"78d1f829d3a1b4c5e6f708192a3b4c5d", "78d1f829d3a1"},
		{"78d1f829", "78d1f829"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := ShortDigest(tt.in); got != tt.want {
			t.Errorf("ShortDigest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTimeHelpersPassThroughGarbage(t *testing.T) {
	if got := DateOf("last tuesday"); got != "last tuesday" {
		t.Errorf("DateOf = %q, want the input back", got)
	}
	if got := FormatTime("last tuesday"); got != "last tuesday" {
		t.Errorf("FormatTime = %q, want the input back", got)
	}
	if got := DateOf(""); got != "" {
		t.Errorf("DateOf(\"\") = %q, want empty", got)
	}
	if got := FormatTime(""); got != "" {
		t.Errorf("FormatTime(\"\") = %q, want empty", got)
	}
	if got := Ago("last tuesday", time.Now()); got != "" {
		t.Errorf("Ago = %q, want empty", got)
	}
	if got := Timestamp("", time.Now()); got != "" {
		t.Errorf("Timestamp(\"\") = %q, want empty", got)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct{ in, want string }{
		{"2026-08-24T11:59:30Z", "just now"},
		{"2026-08-24T11:59:00Z", "1 minute ago"},
		{"2026-08-24T11:30:00Z", "30 minutes ago"},
		{"2026-08-24T11:00:00Z", "1 hour ago"},
		{"2026-08-24T00:00:00Z", "12 hours ago"},
		{"2026-08-23T11:00:00Z", "1 day ago"},
		{"2026-08-13T09:12:00Z", "11 days ago"},
		{"2026-08-24T13:00:00Z", ""},
	}

	for _, tt := range tests {
		if got := Ago(tt.in, now); got != tt.want {
			t.Errorf("Ago(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTimestamp(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	if got, want := Timestamp("2026-08-13T09:12:00Z", now), "2026-08-13 09:12:00 UTC (11 days ago)"; got != want {
		t.Errorf("Timestamp = %q, want %q", got, want)
	}
	// A timestamp in the future gets no age suffix rather than a nonsense one.
	if got, want := Timestamp("2026-08-25T09:12:00Z", now), "2026-08-25 09:12:00 UTC"; got != want {
		t.Errorf("Timestamp = %q, want %q", got, want)
	}
}
