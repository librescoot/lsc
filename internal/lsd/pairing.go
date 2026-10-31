package lsd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

type pairingStatus struct {
	State     string `json:"state"`
	Running   bool   `json:"running"`
	Code      string `json:"code,omitempty"`
	URL       string `json:"url,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

var pairingCode = regexp.MustCompile(`^[0-9ABCDEFGHJKMNPQRSTVWXYZ]{5}-[0-9ABCDEFGHJKMNPQRSTVWXYZ]{5}$`)

func sanitizePairing(status pairingStatus) (pairingStatus, error) {
	switch status.State {
	case "not_registered", "registered", "starting", "waiting_for_web_setup", "applying", "connected", "error", "stopping", "paused":
	default:
		return pairingStatus{}, errors.New("invalid pairing status")
	}
	status.Error = ""
	if status.State == "error" {
		status.Error = "Pairing stopped before completion. Check connectivity and keep the scooter stationary, then retry."
	}
	if status.State != "waiting_for_web_setup" || !pairingCode.MatchString(status.Code) || status.ExpiresAt <= time.Now().Unix() {
		status.Code = ""
		status.URL = ""
		status.ExpiresAt = 0
	} else {
		status.URL = "https://connect.librescoot.org/pair"
	}
	return status, nil
}

func (s *Server) runPairing(ctx context.Context, action string) (pairingStatus, error) {
	command := exec.CommandContext(ctx, "/usr/bin/connect", "-state-dir", filepath.Join(s.dataDir, "connect-service"), "-json", action)
	data, err := command.Output()
	if err != nil || len(data) > 4096 {
		return pairingStatus{}, errors.New("Connect CLI unavailable or pairing state could not be read")
	}
	var status pairingStatus
	if json.Unmarshal(data, &status) != nil {
		return pairingStatus{}, errors.New("Connect CLI returned an invalid response")
	}
	return sanitizePairing(status)
}

func (s *Server) handleCloudPairing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	action := "status"
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		var body struct {
			Action string `json:"action"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		if body.Action != "start" && body.Action != "stop" {
			writeErr(w, http.StatusBadRequest, "action must be start or stop")
			return
		}
		action = body.Action
	default:
		w.Header().Set("Allow", "GET, POST")
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	run := s.pairingRun
	if run == nil {
		run = s.runPairing
	}
	status, err := run(ctx, action)
	if err == nil {
		status, err = sanitizePairing(status)
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "Connect is unavailable. Check that the current Connect CLI and radio-gaga are installed.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}
