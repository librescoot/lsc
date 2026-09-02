package lsd

import (
	"fmt"
	"net/http"
	"time"

	"librescoot/lsc/internal/redis"
)

// statusHashes are the hashes the dashboard renders, copied raw: the UI
// knows the interesting fields, and keeping the payload schema-free means
// new firmware fields show up without an lsd change.
var statusHashes = []string{
	"vehicle",
	"engine-ecu",
	"battery:0",
	"battery:1",
	"aux-battery",
	"cb-battery",
	"power-manager",
	"power-mux",
	"system",
	"scooter",
	"gps",
	"internet",
	"modem",
	"alarm",
	"dashboard",
	"keycard",
	"navigation",
	"ota",
	"maps",
	"settings",
	"version:mdb",
	"version:dbc",
}

// faultSets are the active-fault sets, keyed by the hash whose pub/sub
// channel announces changes to them (payload "fault").
var faultSets = []string{"vehicle", "engine-ecu", "battery:0", "battery:1"}

// statusSnapshot is the aggregate state the dashboard renders.
type statusSnapshot struct {
	Hashes  map[string]map[string]string `json:"hashes"`
	Faults  map[string][]string          `json:"faults"`
	RedisOK bool                         `json:"redis-ok"`
	TS      int64                        `json:"ts"`
}

// snapshot gathers everything the status view needs. Missing hashes are
// omitted, not errors: boards differ (battery:1, CBB, modem are optional).
func (s *Server) snapshot() *statusSnapshot {
	client := s.getRedis()
	if client == nil {
		return nil
	}
	snap := &statusSnapshot{
		Hashes: map[string]map[string]string{},
		Faults: map[string][]string{},
		TS:     time.Now().Unix(),
	}
	for _, key := range statusHashes {
		if m, err := client.HGetAll(key); err == nil && len(m) > 0 {
			snap.Hashes[key] = m
		}
	}
	for hash, members := range collectFaults(client) {
		snap.Faults[hash] = members
	}
	snap.RedisOK = true
	return snap
}

// collectFaults reads every non-empty fault set.
func collectFaults(client *redis.Client) map[string][]string {
	out := map[string][]string{}
	for _, hash := range faultSets {
		if members, err := client.SMembers(hash + ":fault"); err == nil && len(members) > 0 {
			out[hash] = members
		}
	}
	return out
}

// handleStatus answers GET /api/status with the snapshot.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	snap := s.snapshot()
	if snap == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "redis not connected", "redis-ok": false,
		})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleFaults answers GET /api/faults with the raw fault sets.
func (s *Server) handleFaults(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	writeJSON(w, http.StatusOK, collectFaults(client))
}

// handleEvents returns recent entries from the events:faults stream. Each
// entry carries group, code and description plus the stream id, whose
// millisecond prefix is the timestamp.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	msgs, err := client.XRevRangeN(r.Context(), "events:faults", "+", "-", 50)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "read events:faults: "+err.Error())
		return
	}
	out := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		entry := map[string]string{"id": m.ID}
		for k, v := range m.Values {
			entry[k] = fmt.Sprintf("%v", v)
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}
