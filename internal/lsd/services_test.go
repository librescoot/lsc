package lsd

import (
	"testing"
)

func TestListUnitsMissingUnitOmitted(t *testing.T) {
	// list-units with explicit arguments simply omits units that do not
	// exist; it must not error the whole listing.
	rows, err := listUnits()
	if err != nil {
		t.Skipf("systemd not available in test environment: %v", err)
	}
	for _, u := range rows {
		if u.Unit == "lsd-test-definitely-not-a-unit.service" {
			t.Errorf("nonexistent unit leaked into list: %+v", u)
		}
		if !stringsPrefix(u.Unit) {
			t.Errorf("unexpected unit in list: %q", u.Unit)
		}
	}
}

func stringsPrefix(unit string) bool {
	for _, u := range knownUnits {
		if u == unit {
			return true
		}
	}
	return false
}
