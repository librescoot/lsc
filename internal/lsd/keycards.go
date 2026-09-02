package lsd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	rdb "github.com/redis/go-redis/v9"
)

// keycardDataDir is keycard-service's -data-dir default; the unit file passes
// no override. The files hold one uppercase hex UID per line.
const keycardDataDir = "/data/keycard"

var uidRe = regexp.MustCompile(`^[0-9A-F]{2,20}$`)

// normalizeUID accepts the separators people type (04:A1:B2, 04 a1 b2) and
// returns the contiguous uppercase form keycard-service stores.
func normalizeUID(s string) (string, error) {
	u := strings.ToUpper(strings.NewReplacer(":", "", "-", "", " ", "").Replace(strings.TrimSpace(s)))
	if !uidRe.MatchString(u) || len(u)%2 != 0 {
		return "", fmt.Errorf("a card UID is 1 to 10 bytes of hex, like 04A1B2C3")
	}
	return u, nil
}

func readUIDFile(name string) []string {
	data, err := os.ReadFile(filepath.Join(keycardDataDir, name))
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if u, err := normalizeUID(line); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// handleKeycards answers GET /api/keycards with both lists and the last card
// the reader saw (the keycard hash, which expires ten seconds after a tap).
func (s *Server) handleKeycards(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	resp := map[string]interface{}{
		"authorized": readUIDFile("authorized_uids.txt"),
		"master":     readUIDFile("master_uids.txt"),
	}
	if client := s.getRedis(); client != nil {
		if m, err := client.HGetAll("keycard"); err == nil && len(m) > 0 {
			resp["last"] = m
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// keycardCommands are the scooter:keycard commands the page may send. Those
// taking a UID get it appended after validation; nothing else from the
// request reaches the queue.
var keycardCommands = map[string]bool{
	"add": true, "remove": true, "set-master": true,
	"learn:start": false, "learn:stop": false, "learn:master:start": false, "learn:master:stop": false,
	"reset": false,
}

// handleKeycardCommand posts one command and waits for keycard-service's
// answer in keycard[command-result]. The subscription is opened before the
// push so a fast reply cannot slip past.
func (s *Server) handleKeycardCommand(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	var req struct {
		Command string `json:"command"`
		UID     string `json:"uid"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	takesUID, ok := keycardCommands[req.Command]
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("unknown keycard command %q", req.Command))
		return
	}
	payload := req.Command
	if takesUID {
		uid, err := normalizeUID(req.UID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		payload += ":" + uid
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	sub := client.Subscribe(ctx, "keycard")
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		writeErr(w, http.StatusBadGateway, "subscribe: "+err.Error())
		return
	}
	msgs := sub.Channel()

	if err := client.LPush("scooter:keycard", payload); err != nil {
		writeErr(w, http.StatusBadGateway, "push failed: "+err.Error())
		return
	}

	result, err := awaitCommandResult(ctx, msgs, func() (string, error) { return client.HGet("keycard", "command-result") })
	resp := map[string]interface{}{
		"command":    payload,
		"result":     result,
		"authorized": readUIDFile("authorized_uids.txt"),
		"master":     readUIDFile("master_uids.txt"),
	}
	switch {
	case err != nil:
		resp["error"] = "keycard-service did not answer; is librescoot-keycard running?"
		writeJSON(w, http.StatusGatewayTimeout, resp)
	case strings.HasPrefix(result, "error:"):
		resp["error"] = strings.TrimPrefix(result, "error:")
		writeJSON(w, http.StatusUnprocessableEntity, resp)
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}

// awaitCommandResult waits for a command-result notification and reads the
// value.
func awaitCommandResult(ctx context.Context, msgs <-chan *rdb.Message, get func() (string, error)) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", errors.New("timeout")
		case m, ok := <-msgs:
			if !ok {
				return "", errors.New("subscription closed")
			}
			if m.Payload != "command-result" {
				continue
			}
			v, err := get()
			if err != nil {
				return "", err
			}
			return v, nil
		}
	}
}
