package lsd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"librescoot/lsc/internal/redis"
)

type pairingBridgeFixture struct {
	server    *Server
	client    *redis.Client
	miniredis *miniredis.Miniredis
	done      chan struct{}
}

func newPairingBridgeFixture(t *testing.T, run func(context.Context, string) (pairingStatus, error)) *pairingBridgeFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s.rdb = client
	s.pairingRun = run
	f := &pairingBridgeFixture{server: s, client: client, miniredis: mr, done: make(chan struct{})}
	go func() { defer close(f.done); s.runPairingBridge() }()
	t.Cleanup(func() {
		s.Shutdown()
		select {
		case <-f.done:
		case <-time.After(3 * time.Second):
			t.Error("bridge failed to shut down")
		}
		client.Close()
	})
	return f
}

func (f *pairingBridgeFixture) push(t *testing.T, n int, method string, deadline time.Time) {
	t.Helper()
	id := fmt.Sprintf("%032x", n)
	req := pairingRPCRequest{ID: id, Method: method, Reply: pairingChannel + ":reply:" + id, Deadline: deadline.UnixMilli(), Payload: json.RawMessage(`{}`)}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.GetClient().LPush(context.Background(), pairingChannel, raw).Err(); err != nil {
		t.Fatal(err)
	}
}

func waitQueueDrained(t *testing.T, f *pairingBridgeFixture) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		length, err := f.client.GetClient().LLen(context.Background(), pairingChannel).Result()
		if err != nil {
			t.Fatal(err)
		}
		if length == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("bridge did not dequeue requests")
}

func waitPairingAction(t *testing.T, ch <-chan string, expected string) {
	t.Helper()
	select {
	case action := <-ch:
		if action != expected {
			t.Fatalf("got %s, want %s", action, expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", expected)
	}
}

func TestPairingMutationsKeepFIFOOrder(t *testing.T) {
	actions := make(chan string, 4)
	release := make(chan struct{})
	var starts atomic.Int32
	f := newPairingBridgeFixture(t, func(ctx context.Context, action string) (pairingStatus, error) {
		actions <- action
		if action == "start" && starts.Add(1) == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return pairingStatus{}, ctx.Err()
			}
		}
		return pairingStatus{State: "starting", Running: true}, nil
	})
	f.miniredis.HSet("vehicle", "state", "parked")
	f.push(t, 1, "start", time.Now().Add(5*time.Second))
	waitPairingAction(t, actions, "start")
	f.push(t, 2, "stop", time.Now().Add(5*time.Second))
	f.push(t, 3, "start", time.Now().Add(5*time.Second))
	close(release)
	waitPairingAction(t, actions, "stop")
	waitPairingAction(t, actions, "start")
}

func TestPairingQueuedDeadlinesAndCurrentSafety(t *testing.T) {
	actions := make(chan string, 4)
	release := make(chan struct{})
	f := newPairingBridgeFixture(t, func(ctx context.Context, action string) (pairingStatus, error) {
		actions <- action
		if action == "stop" {
			select {
			case <-release:
			case <-ctx.Done():
				return pairingStatus{}, ctx.Err()
			}
		}
		return pairingStatus{State: "paused"}, nil
	})
	f.miniredis.HSet("vehicle", "state", "parked")
	f.push(t, 1, "stop", time.Now().Add(5*time.Second))
	waitPairingAction(t, actions, "stop")
	f.push(t, 2, "stop", time.Now().Add(500*time.Millisecond))
	f.push(t, 3, "start", time.Now().Add(5*time.Second))
	waitQueueDrained(t, f)
	time.Sleep(600 * time.Millisecond)
	f.miniredis.HSet("vehicle", "state", "ready-to-drive")
	close(release)
	// A later stop is a FIFO barrier: neither the expired stop nor unsafe start may run.
	f.push(t, 4, "stop", time.Now().Add(5*time.Second))
	waitPairingAction(t, actions, "stop")
	select {
	case action := <-actions:
		t.Fatalf("unexpected extra action %s", action)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPairingFloodIsBoundedAndShutdownCancelsWorkers(t *testing.T) {
	statusEntered := make(chan struct{})
	startEntered := make(chan struct{})
	var statusCount, active, peak atomic.Int32
	f := newPairingBridgeFixture(t, func(ctx context.Context, action string) (pairingStatus, error) {
		if action == "status" {
			if statusCount.Add(1) == 1 {
				close(statusEntered)
			}
		} else {
			n := active.Add(1)
			if n > peak.Load() {
				peak.Store(n)
			}
			defer active.Add(-1)
			select {
			case <-startEntered:
			default:
				close(startEntered)
			}
		}
		<-ctx.Done()
		return pairingStatus{}, ctx.Err()
	})
	f.miniredis.HSet("vehicle", "state", "parked")
	f.push(t, 1, "status", time.Now().Add(10*time.Second))
	select {
	case <-statusEntered:
	case <-time.After(time.Second):
		t.Fatal("status did not start")
	}
	for i := 2; i < 102; i++ {
		f.push(t, i, "status", time.Now().Add(10*time.Second))
	}
	f.push(t, 102, "start", time.Now().Add(10*time.Second))
	select {
	case <-startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("status flood blocked mutation")
	}
	channel := pairingChannel + ":reply:" + fmt.Sprintf("%032x", 152)
	sub := f.client.GetClient().Subscribe(context.Background(), channel)
	defer sub.Close()
	if _, err := sub.Receive(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 103; i < 153; i++ {
		f.push(t, i, "stop", time.Now().Add(10*time.Second))
	}
	// Drain proves the dispatcher can reject overflow without launching more work.
	deadline := time.Now().Add(2 * time.Second)
	for f.client.GetClient().LLen(context.Background(), pairingChannel).Val() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.client.GetClient().LLen(context.Background(), pairingChannel).Val() != 0 {
		t.Fatal("flood did not drain")
	}
	replyCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message, err := sub.ReceiveMessage(replyCtx)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(message.Payload), &reply); err != nil || reply.OK || reply.Error != "pairing unavailable or scooter not stationary" {
		t.Fatalf("unexpected overflow response: %s (%v)", message.Payload, err)
	}
	if statusCount.Load() != 1 || peak.Load() != 1 {
		t.Fatalf("unexpected concurrency: status %d mutation %d", statusCount.Load(), peak.Load())
	}
	f.server.Shutdown()
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel work")
	}
}
