package service

import "testing"

// Both datastore names must land on the same unit, whichever one this image
// ships. Librescoot 1.2 renamed redis.service to valkey.service; an old script
// saying "redis" has to keep working, and a new one saying "valkey" has to work
// on a pre-1.2 image.
func TestDatastoreAliasesResolveTogether(t *testing.T) {
	redis := resolveServiceName("redis")
	valkey := resolveServiceName("valkey")

	if redis != valkey {
		t.Errorf("resolveServiceName(redis) = %q, resolveServiceName(valkey) = %q, want equal", redis, valkey)
	}
	if redis != "redis" && redis != "valkey" {
		t.Errorf("datastore resolved to %q, want redis or valkey", redis)
	}
}

func TestResolveServiceName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"vehicle", "librescoot-vehicle"},
		{"vehicle.service", "librescoot-vehicle"},
		{"librescoot-vehicle", "librescoot-vehicle"},
		{"backlight", "dbc-backlight"},
		{"something-else", "something-else"},
	}

	for _, tt := range tests {
		if got := resolveServiceName(tt.in); got != tt.want {
			t.Errorf("resolveServiceName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnsureServiceSuffix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"vehicle", "librescoot-vehicle.service"},
		{"vehicle.service", "librescoot-vehicle.service"},
		{"radio-gaga", "radio-gaga.service"},
	}

	for _, tt := range tests {
		if got := ensureServiceSuffix(tt.in); got != tt.want {
			t.Errorf("ensureServiceSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
