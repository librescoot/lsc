package lsd

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// streamChannels are the pub/sub channels the SSE bridge forwards to
// browsers. Motion channels are deliberately absent: at 10 Hz they would
// flood the browser for near-zero value on a management page.
var streamChannels = []string{
	"vehicle",
	"alarm",
	"battery:0",
	"battery:1",
	"aux-battery",
	"cb-battery",
	"engine-ecu",
	"power-manager",
	"power-mux",
	"gps",
	"internet",
	"modem",
	"settings",
	"system",
	"scooter",
	"dashboard",
	"keycard",
	"keycard:events",
	"navigation",
	"ota",
	"maps",
	"version:mdb",
	"version:dbc",
}

// hub fans stream events out to SSE clients.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	closed  bool
}

func newHub() *hub {
	return &hub{clients: make(map[chan []byte]struct{})}
}

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return ch
	}
	h.clients[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

func (h *hub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Client too slow: drop the update rather than back up Redis.
			// The next full snapshot (reconnect) heals it.
		}
	}
}

func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.clients {
		delete(h.clients, ch)
		close(ch)
	}
}

// streamEvent is one SSE "patch" event. Redis pub/sub only says "field F of
// hash H changed", so the bridge reads the new value and ships it along:
// the browser patches its copy of the hash instead of re-fetching the whole
// snapshot. A fault-set change carries the whole set.
type streamEvent struct {
	Hash  string   `json:"h"`
	Field string   `json:"f"`
	Value *string  `json:"v,omitempty"`
	Set   []string `json:"set,omitempty"`
	TS    int64    `json:"ts"`
}

// handleStream bridges Redis pub/sub to Server-Sent Events.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("retry: 3000\n: lsd stream open\n\n"))
	flusher.Flush()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	// Prime the stream with a status snapshot so the UI renders instantly.
	if snap := s.snapshot(); snap != nil {
		if b, err := json.Marshal(snap); err == nil {
			_, _ = w.Write([]byte("event: status\ndata: " + string(b) + "\n\n"))
			flusher.Flush()
		}
	}

	ctx := r.Context()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// runStreamBridge subscribes to Redis and pushes patch events into the hub
// until the server stops or Redis goes away.
func (s *Server) runStreamBridge() {
	for {
		if s.isStopping() {
			return
		}
		client := s.getRedis()
		if client == nil {
			select {
			case <-s.done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		pubsub := client.Subscribe(ctx, streamChannels...)
		msgs := pubsub.Channel()
		log.Printf("Stream bridge subscribed to %d channels", len(streamChannels))

	Loop:
		for {
			select {
			case msg, ok := <-msgs:
				if !ok {
					break Loop
				}
				s.hub.broadcast(s.patchEvent(msg.Channel, msg.Payload))
			case <-s.done():
				cancel()
				_ = pubsub.Close()
				return
			}
		}
		cancel()
		_ = pubsub.Close()
		if s.isStopping() {
			return
		}
		log.Printf("Stream bridge lost Redis, reconnecting in 2s")
		time.Sleep(2 * time.Second)
	}
}

// patchEvent turns a pub/sub notification into an SSE frame carrying the
// new value. A payload of "fault" on hash H means the set H:fault changed.
// keycard:events is not a hash: its payload is the event itself and is
// forwarded as the field with no value.
func (s *Server) patchEvent(hash, field string) []byte {
	ev := streamEvent{Hash: hash, Field: field, TS: time.Now().Unix()}
	if hash == "keycard:events" {
		b, _ := json.Marshal(ev)
		return []byte("data: " + string(b) + "\n\n")
	}
	if client := s.getRedis(); client != nil {
		if field == "fault" {
			members, err := client.SMembers(hash + ":fault")
			if err == nil {
				if members == nil {
					members = []string{}
				}
				ev.Set = members
			}
		} else if v, err := client.HGet(hash, field); err == nil {
			ev.Value = &v
		}
	}
	b, _ := json.Marshal(ev)
	return []byte("data: " + string(b) + "\n\n")
}
