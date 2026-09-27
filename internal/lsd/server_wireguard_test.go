package lsd

import (
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestFirstGlobalIPv4(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
		&net.IPNet{IP: net.ParseIP("169.254.1.2"), Mask: net.CIDRMask(16, 32)},
		&net.IPNet{IP: net.ParseIP("10.7.0.4"), Mask: net.CIDRMask(16, 32)},
	}
	if got := firstGlobalIPv4(addresses); got != "10.7.0.4" {
		t.Fatalf("firstGlobalIPv4 = %q, want 10.7.0.4", got)
	}
	if got := firstGlobalIPv4(addresses[:2]); got != "" {
		t.Fatalf("firstGlobalIPv4 without a usable address = %q", got)
	}
}

func TestWireGuardListenerFollowsAddress(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: s.routes()}
	s.httpServer = srv
	t.Cleanup(s.Shutdown)

	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserved.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	_ = reserved.Close()

	var ip atomic.Value
	ip.Store("127.0.0.1")
	go s.followAddress(srv, port, func() string { return ip.Load().(string) }, 10*time.Millisecond)
	waitForListener(t, addr, true)
	ip.Store("")
	waitForListener(t, addr, false)
	ip.Store("127.0.0.1")
	waitForListener(t, addr, true)
}

func waitForListener(t *testing.T, addr string, available bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if conn != nil {
			_ = conn.Close()
		}
		if (err == nil) == available {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener %s available = %v, want %v", addr, !available, available)
}
