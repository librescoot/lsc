package schema

import "testing"

func TestLookupMatchesIndexedSetting(t *testing.T) {
	want := Setting{Type: "string", Pattern: "indexed", Description: "Location label"}
	s := &Schema{Settings: map[string]Setting{
		"dashboard.saved-locations.0.label": want,
	}}

	got, ok := s.Lookup("dashboard.saved-locations.12.label")
	if !ok {
		t.Fatal("Lookup did not match indexed setting")
	}
	if got.Type != want.Type || got.Pattern != want.Pattern || got.Description != want.Description {
		t.Fatalf("Lookup = %#v, want %#v", got, want)
	}
	if _, ok := s.Lookup("dashboard.saved-locations.nope.label"); ok {
		t.Fatal("Lookup matched a non-numeric index")
	}
}
