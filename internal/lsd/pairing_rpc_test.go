package lsd

import (
	"context"
	"encoding/json"
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
