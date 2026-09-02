package lsd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"librescoot/lsc/internal/redis"
)

// update-service downloads into <data>/ota/<board>. The DBC's update-service
// runs on the DBC with its own /data, so files for it are staged here first
// and pushed over usb0 when an install is requested.
func (s *Server) otaDir() string { return filepath.Join(s.dataDir, "ota") }

// dbcDataServer is the DBC's data-server, which accepts PUT uploads into
// its /data. Only reachable while the dashboard is powered.
const dbcDataServer = "http://192.168.7.2:8080"

var otaBoards = map[string]bool{"mdb": true, "dbc": true}
var otaFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]+\.(mender|delta)$`)

type otaFile struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

func (s *Server) listOTAFiles(board string) []otaFile {
	entries, err := os.ReadDir(filepath.Join(s.otaDir(), board))
	if err != nil {
		return []otaFile{}
	}
	out := []otaFile{}
	for _, e := range entries {
		if e.IsDir() || !otaFileRe.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, otaFile{Name: e.Name(), Size: info.Size(), MTime: info.ModTime().Unix()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MTime > out[j].MTime })
	return out
}

// handleUpdates answers GET /api/updates: the ota hash, per-board versions,
// the updates.* settings and the staged files.
func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	ota, _ := client.HGetAll("ota")
	settings, _ := client.HGetAll("settings")
	upd := map[string]string{}
	for k, v := range settings {
		if strings.HasPrefix(k, "updates.") {
			upd[k] = v
		}
	}
	versions := map[string]map[string]string{}
	for _, b := range []string{"mdb", "dbc"} {
		if m, err := client.HGetAll("version:" + b); err == nil {
			versions[b] = m
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ota":      ota,
		"settings": upd,
		"versions": versions,
		"files":    map[string][]otaFile{"mdb": s.listOTAFiles("mdb"), "dbc": s.listOTAFiles("dbc")},
	})
}

// handleUpdatesUpload stores a .mender or .delta under /data/ota/<board>/
// through the atomic write path and returns its checksum, so the install
// that follows can hand update-service a verified file.
func (s *Server) handleUpdatesUpload(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPut) {
		return
	}
	board := r.URL.Query().Get("board")
	name := filepath.Base(r.URL.Query().Get("name"))
	if !otaBoards[board] {
		writeErr(w, http.StatusBadRequest, "board must be mdb or dbc")
		return
	}
	if !otaFileRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "file must be a .mender or .delta artifact")
		return
	}
	dst := filepath.Join(s.otaDir(), board, name)
	h := sha256.New()
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := writeFileAtomic(dst, io.TeeReader(r.Body, h), 0o644); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, http.StatusRequestEntityTooLarge, "upload exceeds 2 GiB")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored", "board": board, "name": name, "path": dst, "sha256": hex.EncodeToString(h.Sum(nil))})
}

// handleUpdatesAction drives update-service: check-now, preview-channel,
// channel switch (a settings write), install of a staged file, delete of a
// staged file.
func (s *Server) handleUpdatesAction(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	var req struct {
		Board   string `json:"board"`
		Action  string `json:"action"`
		Channel string `json:"channel"`
		File    string `json:"file"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !otaBoards[req.Board] {
		writeErr(w, http.StatusBadRequest, "board must be mdb or dbc")
		return
	}
	queue := "scooter:update:" + req.Board
	switch req.Action {
	case "check":
		if err := client.LPush(queue, "check-now"); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
	case "preview", "channel":
		if req.Channel != "stable" && req.Channel != "testing" && req.Channel != "nightly" {
			writeErr(w, http.StatusBadRequest, "channel must be stable, testing or nightly")
			return
		}
		if req.Action == "preview" {
			if err := client.LPush(queue, "preview-channel:"+req.Channel); err != nil {
				writeErr(w, http.StatusBadGateway, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
			return
		}
		key := "updates." + req.Board + ".channel"
		if err := client.HSet("settings", key, req.Channel); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		_ = client.Publish(r.Context(), "settings", key)
		writeJSON(w, http.StatusOK, map[string]string{"status": "set", "channel": req.Channel})
	case "delete":
		name := filepath.Base(req.File)
		if !otaFileRe.MatchString(name) {
			writeErr(w, http.StatusBadRequest, "not an update file")
			return
		}
		if err := os.Remove(filepath.Join(s.otaDir(), req.Board, name)); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case "install":
		s.installUpdate(w, r, client, req.Board, filepath.Base(req.File))
	default:
		writeErr(w, http.StatusBadRequest, "unknown action")
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// installUpdate hands a staged file to the board's update-service. For the
// MDB that is one queue push. The DBC runs its own update-service against
// its own /data, so its file is first copied over usb0 through the DBC's
// data-server, powering the dashboard on if needed; the queue push then
// names the path on the DBC. The copy can take a minute for a full image.
func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request, client *redis.Client, board, name string) {
	if !otaFileRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "not an update file")
		return
	}
	local := filepath.Join(s.otaDir(), board, name)
	if _, err := os.Stat(local); err != nil {
		writeErr(w, http.StatusNotFound, "no such staged file")
		return
	}
	sum, err := fileSHA256(local)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	target := local
	if board == "dbc" {
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
		defer cancel()
		if err := pushToDBC(ctx, client, local, name); err != nil {
			writeErr(w, http.StatusBadGateway, "copy to dashboard: "+err.Error())
			return
		}
		target = "/data/ota/dbc/" + name
	}
	payload := fmt.Sprintf("update-from-file:%s#sha256=%s", target, sum)
	if err := client.LPush("scooter:update:"+board, payload); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued", "file": target, "sha256": sum})
}

// pushToDBC copies a file into the DBC's /data/ota/dbc via its data-server,
// waking the dashboard first when it is off.
func pushToDBC(ctx context.Context, client *redis.Client, local, name string) error {
	probe := &http.Client{Timeout: 5 * time.Second}
	reachable := func() bool {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, dbcDataServer+"/", nil)
		resp, err := probe.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode < 500
	}
	if !reachable() {
		if err := client.LPush("scooter:hardware", "dashboard:on"); err != nil {
			return err
		}
		deadline := time.Now().Add(2 * time.Minute)
		for !reachable() {
			if time.Now().After(deadline) {
				return errors.New("the dashboard did not come up within two minutes")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
	}
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, dbcDataServer+"/ota/dbc/"+name, f)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("dashboard data-server answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
