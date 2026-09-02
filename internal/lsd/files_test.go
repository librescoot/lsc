package lsd

import (
	"strings"
	"testing"
)

func TestCleanRelPath(t *testing.T) {
	s := &Server{dataDir: "/data"}
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"logs", "logs"},
		{"/logs/", "logs"},
		{"/data/radio-gaga/config.yaml", "data/radio-gaga/config.yaml"},
		{"a//b.txt", "a/b.txt"},
		// Traversal collapses to a path inside the data dir, never out of it.
		{"../escape", "escape"},
		{"a/../../escape", "escape"},
		{"..", ""},
	}
	for _, tt := range tests {
		got := s.cleanRelPath(tt.in)
		if got != tt.want {
			t.Errorf("cleanRelPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.HasPrefix(got, "/") || strings.Contains(got, "..") {
			t.Errorf("cleanRelPath(%q) = %q escapes the data dir", tt.in, got)
		}
	}
}

func TestContentDisposition(t *testing.T) {
	got := contentDisposition(`we"rd.txt`)
	if !strings.HasPrefix(got, `attachment; filename="we_rd.txt"`) {
		t.Errorf("contentDisposition = %q", got)
	}
}
