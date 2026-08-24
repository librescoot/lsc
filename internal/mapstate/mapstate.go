// Package mapstate reads the `maps` Redis hash, which records the offline map
// and routing tiles installed on the dashboard.
//
// The hash lives on the MDB but describes files under the DBC's /data, so
// anything on the MDB can tell what a vehicle has without powering the
// dashboard up. It is written by the installer at provisioning time and
// rewritten by the dashboard, which is authoritative once it runs.
package mapstate

import (
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/format"
)

// Hash is the Redis hash name.
const Hash = "maps"

// Artifact describes one installed tile file.
type Artifact struct {
	// Installed mirrors the presence of the `<prefix>:size` field, which is
	// the presence test for the artifact: a file that is not on disk gets none
	// of its four fields written.
	Installed bool
	// Size is the file size in bytes. SizeKnown is false when the field held
	// something that is not a number.
	Size      int64
	SizeKnown bool
	// SHA256 and PublishedAt can both be absent on an installed artifact. That
	// is a vehicle whose tiles were flashed or copied in by hand: they are
	// there, but nothing recorded where they came from.
	SHA256      string
	PublishedAt string
	// MTime is when the file was written on the DBC.
	MTime string
}

// HasProvenance reports whether anything recorded where the artifact came from.
func (a Artifact) HasProvenance() bool {
	return a.SHA256 != "" || a.PublishedAt != ""
}

// Summary is a one-line description for dense output.
func (a Artifact) Summary() string {
	if !a.Installed {
		return "not installed"
	}

	var parts []string
	if a.SizeKnown {
		parts = append(parts, format.Bytes(a.Size))
	} else {
		parts = append(parts, "installed")
	}
	if a.SHA256 != "" {
		parts = append(parts, ShortDigest(a.SHA256))
	}
	if d := DateOf(a.PublishedAt); d != "" {
		parts = append(parts, "published "+d)
	} else if d := DateOf(a.MTime); d != "" {
		parts = append(parts, "written "+d)
	}
	if !a.HasProvenance() {
		parts = append(parts, "provenance unknown")
	}
	return strings.Join(parts, ", ")
}

// JSON renders the artifact for --json output.
func (a Artifact) JSON() map[string]any {
	out := map[string]any{
		"installed":    a.Installed,
		"size_bytes":   nil,
		"sha256":       nil,
		"published_at": nil,
		"mtime":        nil,
	}
	if a.SizeKnown {
		out["size_bytes"] = a.Size
	}
	if a.SHA256 != "" {
		out["sha256"] = a.SHA256
	}
	if a.PublishedAt != "" {
		out["published_at"] = a.PublishedAt
	}
	if a.MTime != "" {
		out["mtime"] = a.MTime
	}
	return out
}

// State is a parsed `maps` hash.
type State struct {
	// Recorded is false when the hash does not exist. valkey on the MDB runs
	// without persistence, so an MDB reboot clears it and it only comes back
	// once the dashboard next boots. An unrecorded state says nothing about
	// what is installed on disk.
	Recorded bool
	// Region is the slug, matching the published filenames. It is absent when
	// no install path could name the tiles.
	Region string
	// RegionName is the human-readable form, absent until the dashboard has run.
	RegionName           string
	Map                  Artifact
	Routing              Artifact
	LastUpdateCheck      string
	UpdateAvailable      bool
	UpdateAvailableKnown bool
	UpdatedAt            string
}

// AnyInstalled reports whether either artifact is on disk.
func (s State) AnyInstalled() bool {
	return s.Map.Installed || s.Routing.Installed
}

// RegionLabel is the best available name for the installed region, or the
// empty string when nothing named it.
func (s State) RegionLabel() string {
	switch {
	case s.RegionName != "" && s.Region != "" && s.RegionName != s.Region:
		return s.RegionName + " (" + s.Region + ")"
	case s.RegionName != "":
		return s.RegionName
	default:
		return s.Region
	}
}

// RegionShort is the friendliest single name for the region, without the slug
// in tow, for lines that already carry other text.
func (s State) RegionShort() string {
	if s.RegionName != "" {
		return s.RegionName
	}
	return s.Region
}

// JSON renders the state for --json output. Both `lsc maps` and
// `lsc diag dashboard status` embed this shape.
func (s State) JSON() map[string]any {
	out := map[string]any{
		"recorded":          s.Recorded,
		"region":            nil,
		"region_name":       nil,
		"map":               s.Map.JSON(),
		"routing":           s.Routing.JSON(),
		"last_update_check": nil,
		"update_available":  nil,
		"updated_at":        nil,
	}
	if s.Region != "" {
		out["region"] = s.Region
	}
	if s.RegionName != "" {
		out["region_name"] = s.RegionName
	}
	if s.LastUpdateCheck != "" {
		out["last_update_check"] = s.LastUpdateCheck
	}
	if s.UpdateAvailableKnown {
		out["update_available"] = s.UpdateAvailable
	}
	if s.UpdatedAt != "" {
		out["updated_at"] = s.UpdatedAt
	}
	return out
}

// Parse turns a HGETALL result into a State. An empty map yields a zero State
// with Recorded false.
func Parse(fields map[string]string) State {
	get := func(key string) string {
		return strings.TrimSpace(fields[key])
	}

	s := State{
		Recorded:        len(fields) > 0,
		Region:          get("region"),
		RegionName:      get("region-name"),
		Map:             parseArtifact(get, "map"),
		Routing:         parseArtifact(get, "routing"),
		LastUpdateCheck: get("last-update-check"),
		UpdatedAt:       get("updated-at"),
	}

	switch get("update-available") {
	case "true":
		s.UpdateAvailable, s.UpdateAvailableKnown = true, true
	case "false":
		s.UpdateAvailable, s.UpdateAvailableKnown = false, true
	}

	return s
}

func parseArtifact(get func(string) string, prefix string) Artifact {
	size := get(prefix + ":size")
	a := Artifact{
		Installed:   size != "",
		SHA256:      get(prefix + ":sha256"),
		PublishedAt: get(prefix + ":published-at"),
		MTime:       get(prefix + ":mtime"),
	}
	if v, err := strconv.ParseInt(size, 10, 64); err == nil {
		a.Size, a.SizeKnown = v, true
	}
	return a
}

// ShortDigest trims a hex digest to a git-length prefix.
func ShortDigest(digest string) string {
	const shortLen = 12
	if len(digest) <= shortLen {
		return digest
	}
	return digest[:shortLen]
}

// DateOf renders the calendar date of an ISO8601 timestamp. Anything that does
// not parse is passed through unchanged, so an unexpected format still shows
// up instead of vanishing.
func DateOf(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02")
}

// FormatTime renders an ISO8601 timestamp for display, passing through
// anything that does not parse.
func FormatTime(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04:05 MST")
}

// Ago renders how long before now a timestamp is. It returns the empty string
// for timestamps that do not parse or that lie in the future.
func Ago(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return ""
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}

// Timestamp renders an ISO8601 timestamp with its age, for example
// "2026-08-13 09:12:05 UTC (11 days ago)".
func Timestamp(ts string, now time.Time) string {
	text := FormatTime(ts)
	if text == "" {
		return ""
	}
	if age := Ago(ts, now); age != "" {
		return text + " (" + age + ")"
	}
	return text
}

func plural(n int, unit string) string {
	suffix := "s"
	if n == 1 {
		suffix = ""
	}
	return strconv.Itoa(n) + " " + unit + suffix + " ago"
}
