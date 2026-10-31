package lsd

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudPairingUsesAuthAndOriginGuard(t *testing.T) {
	s, err := New(Options{Token: "management-token", DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	s.pairingRun = func(_ context.Context, action string) (pairingStatus, error) {
		actions = append(actions, action)
		return pairingStatus{State: "starting", Running: true}, nil
	}
	for _, tt := range []struct {
		method, token, origin, body string
		want                        int
	}{
		{"GET", "", "", "", 401},
		{"POST", "management-token", "https://attacker.invalid", `{"action":"start"}`, 403},
		{"POST", "management-token", "", `{"action":"shell"}`, 400},
		{"DELETE", "management-token", "", "", 405},
		{"GET", "management-token", "", "", 200},
		{"POST", "management-token", "", `{"action":"start"}`, 200},
		{"POST", "management-token", "", `{"action":"stop"}`, 200},
	} {
		r := httptest.NewRequest(tt.method, "http://192.168.7.1:8090/api/cloud/pairing", strings.NewReader(tt.body))
		if tt.token != "" {
			r.Header.Set("Authorization", "Bearer "+tt.token)
		}
		if tt.origin != "" {
			r.Header.Set("Origin", tt.origin)
		}
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatalf("%s: got %d want %d", tt.method, w.Code, tt.want)
		}
		if tt.want == 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("pairing response cached")
		}
	}
	if strings.Join(actions, ",") != "status,start,stop" {
		t.Fatalf("unexpected runner calls: %v", actions)
	}
}

func TestCloudPairingPublishesOnlySanitizedView(t *testing.T) {
	s, _ := New(Options{DataDir: t.TempDir()})
	s.pairingRun = func(context.Context, string) (pairingStatus, error) {
		return pairingStatus{State: "waiting_for_web_setup", Running: true, Code: "01234-56789", URL: "https://attacker.invalid/private-token", ExpiresAt: time.Now().Add(time.Minute).Unix(), Error: "private-provider-token"}, nil
	}
	r := httptest.NewRequest("GET", "/api/cloud/pairing", nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://connect.librescoot.org/pair") || strings.Contains(w.Body.String(), "private-") || strings.Contains(w.Body.String(), "attacker") {
		t.Fatal("unsafe pairing response")
	}
	s.pairingRun = func(context.Context, string) (pairingStatus, error) {
		return pairingStatus{}, errors.New("private-token")
	}
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-token") {
		t.Fatal("runner error escaped")
	}
	for _, state := range []pairingStatus{
		{State: "unknown-secret-state"},
		{State: "waiting_for_web_setup", Code: "invalid", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		{State: "waiting_for_web_setup", Code: "01234-56789", ExpiresAt: 1},
		{State: "connected", Code: "01234-56789", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	} {
		view, err := sanitizePairing(state)
		if err == nil && (view.Code != "" || view.URL != "") {
			t.Fatal("invalid or expired code exposed")
		}
	}
}
