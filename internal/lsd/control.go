package lsd

import (
	"fmt"
	"net/http"
	"time"

	"librescoot/lsc/internal/redis"
)

// controlActions maps a UI action to the command queue and payload it
// pushes. Nothing from the request body reaches Redis except through this
// table.
var controlActions = map[string][2]string{
	"lock":            {"scooter:state", "lock"},
	"unlock":          {"scooter:state", "unlock"},
	"power-run":       {"scooter:power", "run"},
	"power-suspend":   {"scooter:power", "suspend"},
	"power-hibernate": {"scooter:power", "hibernate"},
	// Manual hibernation outranks a pending lower-priority target (suspend,
	// timer hibernation), which is what a person pressing the button expects.
	"power-hibernate-manual": {"scooter:power", "hibernate-manual"},
	"power-hibernate-cancel": {"scooter:power", "hibernate-cancel"},
	"power-reboot":           {"scooter:power", "reboot"},
	"seatbox-open":           {"scooter:seatbox", "open"},
	"seatbox-close":          {"scooter:seatbox", "close"},
	"horn-on":                {"scooter:horn", "on"},
	"horn-off":               {"scooter:horn", "off"},
	"blinkers-off":           {"scooter:blinker", "off"},
	"blinkers-left":          {"scooter:blinker", "left"},
	"blinkers-right":         {"scooter:blinker", "right"},
	"blinkers-both":          {"scooter:blinker", "both"},
	"alarm-arm":              {"scooter:alarm", "arm"},
	"alarm-disarm":           {"scooter:alarm", "disarm"},
	"alarm-stop":             {"scooter:alarm", "stop"},
	"alarm-trigger":          {"scooter:alarm", "start:5"},
	"service-mode-on":        {"settings:overlay", "apply:service"},
	"service-mode-off":       {"settings:overlay", "clear:service"},
}

// honkDuration is how long a "honk" keeps the horn on. vehicle-service only
// knows on and off, so the pulse is timed here.
const honkDuration = 400 * time.Millisecond

// handleControl queues one scooter command: POST {"action": "<name>"}.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	var req struct {
		Action  string `json:"action"`
		Seconds int64  `json:"seconds"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Action == "power-hibernate-for" {
		// pm-service clamps to pm.wake-timer-max-seconds itself; the bounds
		// here only reject nonsense before it reaches the queue.
		if req.Seconds < 60 || req.Seconds > 30*24*3600 {
			writeErr(w, http.StatusBadRequest, "seconds must be between 60 and 2592000")
			return
		}
		payload := fmt.Sprintf("hibernate-for:%d", req.Seconds)
		if err := client.LPush("scooter:power", payload); err != nil {
			writeErr(w, http.StatusBadGateway, "push failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "queued", "action": req.Action, "queue": "scooter:power", "payload": payload})
		return
	}
	if req.Action == "honk" {
		if err := client.LPush("scooter:horn", "on"); err != nil {
			writeErr(w, http.StatusBadGateway, "push failed: "+err.Error())
			return
		}
		go hornOffLater(client)
		writeJSON(w, http.StatusOK, map[string]string{"status": "queued", "action": "honk", "queue": "scooter:horn", "payload": "on"})
		return
	}
	a, ok := controlActions[req.Action]
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("unknown action %q", req.Action))
		return
	}
	if err := client.LPush(a[0], a[1]); err != nil {
		writeErr(w, http.StatusBadGateway, "push failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "queued",
		"action":  req.Action,
		"queue":   a[0],
		"payload": a[1],
	})
}

// hornOffLater ends a honk. It retries because a horn left on by a failed
// push is the worst outcome this endpoint can produce.
func hornOffLater(client *redis.Client) {
	time.Sleep(honkDuration)
	for i := 0; i < 3; i++ {
		if err := client.LPush("scooter:horn", "off"); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
