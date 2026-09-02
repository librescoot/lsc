package lsd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	mk := func(hdr map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "http://192.168.7.1:8090/api/control", strings.NewReader("{}"))
		r.Host = "192.168.7.1:8090"
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	tests := []struct {
		name string
		hdr  map[string]string
		want bool
	}{
		{"no headers (curl)", nil, true},
		{"same origin", map[string]string{"Origin": "http://192.168.7.1:8090", "Sec-Fetch-Site": "same-origin"}, true},
		{"same origin, no fetch metadata", map[string]string{"Origin": "http://192.168.7.1:8090"}, true},
		{"cross site", map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, false},
		{"cross site, no fetch metadata", map[string]string{"Origin": "https://evil.example"}, false},
		{"opaque origin", map[string]string{"Origin": "null"}, false},
	}
	for _, tt := range tests {
		if got := sameOrigin(mk(tt.hdr)); got != tt.want {
			t.Errorf("%s: sameOrigin = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGuardBlocksCrossOriginWrites(t *testing.T) {
	s := &Server{}
	called := false
	h := s.guard(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest(http.MethodPost, "/api/control", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h(w, r)
	if called || w.Code != http.StatusForbidden {
		t.Errorf("cross-origin POST: called=%v code=%d", called, w.Code)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	h(w, r)
	if !called {
		t.Error("cross-origin GET must pass (no CORS headers are sent, the browser blocks the read)")
	}
}
