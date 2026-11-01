package lsd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	ipc "github.com/librescoot/redis-ipc"
	"librescoot/lsc/internal/redis"
)

func TestPairingRPCRejectsExpiredCallsArgumentsAndArbitraryReplyChannels(t *testing.T) {
	now := time.Now()
	base := pairingRPCRequest{ID: "12345678-1234-1234-1234-123456789abc", Method: "start", Deadline: now.Add(time.Second).UnixMilli(), Payload: json.RawMessage(`{}`)}
	base.Reply = pairingChannel + ":reply:" + base.ID
	data, _ := json.Marshal(base)
	if _, err := parsePairingRPC(string(data), now); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*pairingRPCRequest){
		func(r *pairingRPCRequest) { r.Deadline = now.Add(-time.Second).UnixMilli() },
		func(r *pairingRPCRequest) { r.Deadline = now.Add(time.Hour).UnixMilli() },
		func(r *pairingRPCRequest) { r.Reply = "vehicle" },
		func(r *pairingRPCRequest) { r.Method = "shell" },
		func(r *pairingRPCRequest) { r.Payload = json.RawMessage(`{"command":"unsafe"}`) },
		func(r *pairingRPCRequest) { r.Payload = json.RawMessage(`null`) },
	} {
		request := base
		change(&request)
		data, _ := json.Marshal(request)
		if _, err := parsePairingRPC(string(data), now); err == nil {
			t.Fatal("invalid RPC accepted")
		}
	}
	if _, err := parsePairingRPC(string(data)+`{}`, now); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestPairingBrowserFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("real-browser test")
	}
	if os.Getenv("LSD_PAIRING_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in real-browser fixture")
	}
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	s, _ := New(Options{DataDir: t.TempDir()})
	var started atomic.Bool
	s.pairingRun = func(_ context.Context, action string) (pairingStatus, error) {
		if action == "start" {
			started.Store(true)
			return pairingStatus{State: "starting", Running: true}, nil
		}
		if action == "stop" {
			started.Store(false)
			return pairingStatus{State: "paused"}, nil
		}
		if started.Load() {
			return pairingStatus{State: "waiting_for_web_setup", Running: true, Code: "01234-56789", ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}, nil
		}
		return pairingStatus{State: "not_registered"}, nil
	}
	s.mu.Lock()
	s.rdb = client
	s.mu.Unlock()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: s.routes()}
	go httpServer.Serve(listener)
	defer httpServer.Close()
	defer client.Close()
	fmt.Printf("PAIRING_FIXTURE_URL=http://%s/#cloud\n", listener.Addr())
	time.Sleep(35 * time.Second)
}

func TestPairingStatusDoesNotDelayMutation(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	s, _ := New(Options{DataDir: t.TempDir()})
	s.mu.Lock()
	s.rdb = client
	s.mu.Unlock()
	mr.HSet("vehicle", "state", "parked")
	statusStarted, releaseStatus, startCalled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.pairingRun = func(ctx context.Context, action string) (pairingStatus, error) {
		if action == "status" {
			close(statusStarted)
			select {
			case <-releaseStatus:
			case <-ctx.Done():
				return pairingStatus{}, ctx.Err()
			}
		}
		if action == "start" {
			close(startCalled)
		}
		return pairingStatus{State: "starting", Running: true}, nil
	}
	done := make(chan struct{})
	go func() { defer close(done); s.runPairingBridge() }()
	defer func() { close(releaseStatus); s.Shutdown(); <-done }()
	push := func(id, method string) {
		request := pairingRPCRequest{ID: id, Method: method, Reply: pairingChannel + ":reply:" + id, Deadline: time.Now().Add(4 * time.Second).UnixMilli(), Payload: json.RawMessage(`{}`)}
		data, _ := json.Marshal(request)
		if err := client.GetClient().LPush(context.Background(), pairingChannel, data).Err(); err != nil {
			t.Fatal(err)
		}
	}
	push("00000000000000000000000000000001", "status")
	select {
	case <-statusStarted:
	case <-time.After(time.Second):
		t.Fatal("status did not start")
	}
	push("00000000000000000000000000000002", "start")
	select {
	case <-startCalled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("start blocked behind status")
	}
}

func TestPairingRPCBridgeUsesCurrentVehicleStateAndSanitizedView(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	s, _ := New(Options{DataDir: t.TempDir()})
	s.mu.Lock()
	s.rdb = client
	s.mu.Unlock()
	var calls atomic.Int32
	s.pairingRun = func(_ context.Context, action string) (pairingStatus, error) {
		calls.Add(1)
		return pairingStatus{State: "starting", Running: true, Error: "private-provider-token"}, nil
	}
	done := make(chan struct{})
	go func() { defer close(done); s.runPairingBridge() }()
	defer func() {
		s.Shutdown()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("RPC consumer did not stop")
		}
	}()
	rpc, err := ipc.New(ipc.WithURL(mr.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.Close()
	mr.HSet("vehicle", "state", "ready-to-drive")
	if _, err := ipc.CallMethod[struct{}, pairingStatus](rpc, pairingChannel, "start", struct{}{}, time.Second); err == nil {
		t.Fatal("start while riding accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("riding request invoked CLI")
	}
	for _, state := range []string{"stand-by", "parked"} {
		mr.HSet("vehicle", "state", state)
		result, err := ipc.CallMethod[struct{}, pairingStatus](rpc, pairingChannel, "start", struct{}{}, time.Second)
		if err != nil || result.State != "starting" || strings.Contains(result.Error, "private-") {
			t.Fatalf("pairing request failed: %v", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("wrong CLI invocation count")
	}
}
