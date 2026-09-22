package locations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"librescoot/lsc/internal/redis"
)

// envelope mirrors the redis-ipc request envelope.
type envelope struct {
	ID           string          `json:"id"`
	Method       string          `json:"method"`
	ReplyChannel string          `json:"reply_channel"`
	Deadline     int64           `json:"deadline"`
	Payload      json.RawMessage `json:"payload"`
}

// startDestinationServer plays settings-service's side of the wire: BRPOP the
// request queue, hand the envelope to the handler, publish its reply.
func startDestinationServer(t *testing.T, addr string,
	handler func(env envelope) []byte) chan envelope {
	t.Helper()
	serverSide := redis.NewClient(addr)
	t.Cleanup(func() { serverSide.Close() })
	rdb := serverSide.GetClient()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	envelopes := make(chan envelope, 8)
	go func() {
		for {
			result, err := rdb.BRPop(ctx, 0, destinationChannel).Result()
			if err != nil {
				return
			}
			var env envelope
			if err := json.Unmarshal([]byte(result[1]), &env); err != nil {
				continue
			}
			envelopes <- env
			if reply := handler(env); reply != nil {
				rdb.Publish(ctx, env.ReplyChannel, reply)
			}
		}
	}()
	return envelopes
}

// newTestRedis points the package client at a throwaway server. destinationCall
// builds its own ipc client from RedisClient.Addr(), so no connection is made
// through this client itself.
func newTestRedis(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	previous := RedisClient
	RedisClient = redis.NewClient(addr)
	t.Cleanup(func() { RedisClient = previous })
	return mr, addr
}

func waitForEnvelope(t *testing.T, envelopes chan envelope) envelope {
	t.Helper()
	select {
	case env := <-envelopes:
		return env
	case <-time.After(3 * time.Second):
		t.Fatal("no destination call arrived")
		return envelope{}
	}
}

func TestSaveLocationAllocatesSlotOverRPC(t *testing.T) {
	_, addr := newTestRedis(t)
	envelopes := startDestinationServer(t, addr, func(envelope) []byte {
		return []byte(`{"ok":true,"payload":{"id":1,"uuid":"3fa85f64-5717-4562-b3fc-2c963f66afa6"}}`)
	})

	id, err := saveLocation(SavedLocation{ID: -1, Latitude: 52.5, Longitude: 13.4, Label: "Home"})
	if err != nil {
		t.Fatalf("saveLocation: %v", err)
	}
	if id != 1 {
		t.Errorf("id = %d, want the server's allocation", id)
	}

	env := waitForEnvelope(t, envelopes)
	if env.Method != "destination.save" {
		t.Errorf("method = %q, want destination.save", env.Method)
	}
	if env.ID == "" {
		t.Error("envelope has no id")
	}
	if !strings.HasPrefix(env.ReplyChannel, destinationChannel+":reply:") {
		t.Errorf("reply channel = %q, want a per-call channel", env.ReplyChannel)
	}
	if env.Deadline <= time.Now().UnixMilli() {
		t.Errorf("deadline = %d, want a future timestamp", env.Deadline)
	}
	var request saveRequest
	if err := json.Unmarshal(env.Payload, &request); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if request.ID != nil {
		t.Errorf("id = %d, want absent so the server allocates", *request.ID)
	}
	if request.Latitude != 52.5 || request.Longitude != 13.4 || request.Label != "Home" {
		t.Errorf("request = %+v, want the record's coordinates and label", request)
	}
}

func TestSaveLocationPassesExplicitID(t *testing.T) {
	_, addr := newTestRedis(t)
	envelopes := startDestinationServer(t, addr, func(envelope) []byte {
		return []byte(`{"ok":true,"payload":{"id":5,"uuid":"0d6c21f0-0000-4000-8000-000000000001"}}`)
	})

	id, err := saveLocation(SavedLocation{ID: 5, Latitude: 1.5, Longitude: 2.5, Label: "Edit"})
	if err != nil {
		t.Fatalf("saveLocation: %v", err)
	}
	if id != 5 {
		t.Errorf("id = %d, want 5", id)
	}
	env := waitForEnvelope(t, envelopes)
	var request saveRequest
	if err := json.Unmarshal(env.Payload, &request); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if request.ID == nil || *request.ID != 5 {
		t.Errorf("id = %v, want explicit 5", request.ID)
	}
}

func TestDeleteLocationCallsDestinationDelete(t *testing.T) {
	_, addr := newTestRedis(t)
	envelopes := startDestinationServer(t, addr, func(envelope) []byte {
		return []byte(`{"ok":true,"payload":{}}`)
	})

	if err := deleteLocation(2); err != nil {
		t.Fatalf("deleteLocation: %v", err)
	}
	env := waitForEnvelope(t, envelopes)
	if env.Method != "destination.delete" {
		t.Errorf("method = %q, want destination.delete", env.Method)
	}
	var request idRequest
	if err := json.Unmarshal(env.Payload, &request); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if request.ID != 2 {
		t.Errorf("id = %d, want 2", request.ID)
	}
}

func TestDestinationCallSurfacesErrors(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		_, addr := newTestRedis(t)
		startDestinationServer(t, addr, func(envelope) []byte {
			return []byte(`{"ok":false,"error":"boom"}`)
		})

		var reply emptyResponse
		err := destinationCall("destination.touch", idRequest{ID: 1}, &reply)
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("err = %v, want the server error", err)
		}
	})

	t.Run("missing client", func(t *testing.T) {
		previous := RedisClient
		RedisClient = nil
		t.Cleanup(func() { RedisClient = previous })

		var reply emptyResponse
		if err := destinationCall("destination.touch", idRequest{ID: 1}, &reply); err == nil {
			t.Error("destinationCall succeeded without a redis client")
		}
	})
}
