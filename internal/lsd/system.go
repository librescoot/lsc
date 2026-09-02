package lsd

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// lsc logs writes its archives to <data>/log-bundles.
func (s *Server) logBundleDir() string { return filepath.Join(s.dataDir, "log-bundles") }

// handleLogBundles lists bundles (GET) or creates one (POST) by running
// lsc logs, which already knows the services, the Redis snapshots and the
// archive layout support expects.
func (s *Server) handleLogBundles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"bundles": s.listBundles()})
	case http.MethodPost:
		var req struct {
			Since string `json:"since"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		since := req.Since
		switch since {
		case "1h", "6h", "24h", "3d", "7d":
		case "":
			since = "24h"
		default:
			writeErr(w, http.StatusBadRequest, "since must be one of 1h, 6h, 24h, 3d, 7d")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		before := map[string]bool{}
		for _, b := range s.listBundles() {
			before[b.Name] = true
		}
		out, err := exec.CommandContext(ctx, "lsc", "logs", "all", "--since", since, "--output", s.logBundleDir()).CombinedOutput()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "lsc logs failed: " + err.Error(), "output": tail(string(out), 2000)})
			return
		}
		created := ""
		for _, b := range s.listBundles() {
			if !before[b.Name] {
				created = b.Name
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "created", "bundle": created, "bundles": s.listBundles()})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listBundles() []otaFile {
	entries, err := os.ReadDir(s.logBundleDir())
	if err != nil {
		return []otaFile{}
	}
	out := []otaFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, otaFile{Name: e.Name(), Size: info.Size(), MTime: info.ModTime().Unix()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MTime > out[j].MTime })
	return out
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// handleJournal returns the last lines of one known unit's journal, the
// whole journal, or the kernel log. Unit names are checked against the same
// list the services page uses, so nothing from the query reaches journalctl
// unvetted.
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	lines := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && n >= 10 && n <= 2000 {
		lines = n
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	unit := r.URL.Query().Get("unit")
	switch unit {
	case "dmesg":
		cmd = exec.CommandContext(ctx, "dmesg", "--time-format=iso")
	case "":
		cmd = exec.CommandContext(ctx, "journalctl", "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	default:
		known := false
		for _, u := range knownUnits {
			if u == unit {
				known = true
			}
		}
		if !known && !(strings.HasPrefix(unit, "librescoot-") && unitNameRe.MatchString(unit)) {
			writeErr(w, http.StatusBadRequest, "unknown unit")
			return
		}
		cmd = exec.CommandContext(ctx, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	}
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if unit == "dmesg" {
		parts := bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n"))
		if len(parts) > lines {
			parts = parts[len(parts)-lines:]
		}
		out = bytes.Join(parts, []byte("\n"))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}
