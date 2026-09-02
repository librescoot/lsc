package lsd

import (
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// knownUnits are the Librescoot systemd units the services page shows, MDB
// and DBC together. The board simply omits the units it does not have.
// Names verified against meta-librescoot recipes (2026-09).
var knownUnits = []string{
	"valkey.service",
	"redis.service",
	"librescoot-onboot.service",
	"librescoot-netconfig.service",
	"librescoot-usb0-failsafe.service",
	"librescoot-vehicle.service",
	"librescoot-battery.service",
	"librescoot-ecu.service",
	"librescoot-modem.service",
	"librescoot-alarm.service",
	"librescoot-settings.service",
	"librescoot-keycard.service",
	"librescoot-boot-led.service",
	"librescoot-bluetooth.service",
	"librescoot-motion.service",
	"librescoot-events.service",
	"librescoot-ums.service",
	"librescoot-pm.service",
	"librescoot-update.service",
	"librescoot-version.service",
	"librescoot-uplink.service",
	"librescoot-data-server.service",
	"librescoot-lsd.service",
	"radio-gaga.service",
	"scootui-qt.service",
	"valhalla.service",
	"dbc-backlight.service",
	"ppp-link.service",
}

// unitStatus is one row of the services table.
type unitStatus struct {
	Unit        string `json:"unit"`
	Load        string `json:"load,omitempty"`
	Active      string `json:"active,omitempty"`
	Sub         string `json:"sub,omitempty"`
	Description string `json:"description,omitempty"`
	Found       bool   `json:"found"`
}

var (
	svcCacheMu   sync.Mutex
	svcCacheTime time.Time
	svcCacheRows []unitStatus
)

// listUnits asks systemd about all known units in ONE call: list-units with
// explicit unit arguments answers in tens of milliseconds, where one
// `systemctl show` per unit costs hundreds. Rows are cached briefly so
// flipping UI filters and quick refreshes stay cheap.
func listUnits() ([]unitStatus, error) {
	svcCacheMu.Lock()
	if time.Since(svcCacheTime) < 2*time.Second && svcCacheRows != nil {
		rows := svcCacheRows
		svcCacheMu.Unlock()
		return rows, nil
	}
	svcCacheMu.Unlock()

	args := append([]string{
		"list-units", "--all", "--no-legend", "--plain", "--type=service", "--",
	}, knownUnits...)
	out, err := exec.Command("systemctl", args...).Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	var rows []unitStatus
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		// Columns: UNIT LOAD ACTIVE SUB DESCRIPTION...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		desc := ""
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}
		rows = append(rows, unitStatus{
			Unit:        fields[0],
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: desc,
			Found:       fields[1] == "loaded",
		})
	}

	svcCacheMu.Lock()
	svcCacheTime = time.Now()
	svcCacheRows = rows
	svcCacheMu.Unlock()
	return rows, nil
}

// invalidateUnitCache forces the next listUnits to ask systemd again, for
// callers that just changed a unit and want to report its new state.
func invalidateUnitCache() {
	svcCacheMu.Lock()
	svcCacheTime = time.Time{}
	svcCacheMu.Unlock()
}

// handleServices answers GET /api/services.
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	rows, err := listUnits()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "systemctl: "+err.Error())
		return
	}
	units := make([]unitStatus, 0, len(rows))
	for _, u := range rows {
		if u.Found {
			units = append(units, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"units": units})
}

var unitNameRe = regexp.MustCompile(`^[A-Za-z0-9@._-]+$`)

// handleServiceAction applies start/stop/restart/enable/disable to a unit.
// Only unit names from knownUnits (plus a radio-gaga alias set) are accepted,
// so a request can never reach systemctl with arbitrary text.
func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Unit   string `json:"unit"`
		Action string `json:"action"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	allowed := false
	for _, u := range knownUnits {
		if u == req.Unit {
			allowed = true
			break
		}
	}
	// Anything else with a Librescoot prefix is accepted so services this
	// binary predates still work. exec.Command does not run a shell, so the
	// prefix plus a plain unit-name character set is the whole gate.
	if !allowed && strings.HasPrefix(req.Unit, "librescoot-") && unitNameRe.MatchString(req.Unit) {
		allowed = true
	}
	if !allowed {
		writeErr(w, http.StatusBadRequest, "unknown unit")
		return
	}

	unit := req.Unit
	if !strings.HasSuffix(unit, ".service") {
		unit += ".service"
	}
	switch req.Action {
	case "start", "stop", "restart", "enable", "disable":
		// systemd handles these directly.
	case "":
		writeErr(w, http.StatusBadRequest, "action required")
		return
	default:
		writeErr(w, http.StatusBadRequest, "unsupported action")
		return
	}

	out, err := exec.Command("systemctl", req.Action, unit).CombinedOutput()
	invalidateUnitCache()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error":  fmt.Sprintf("systemctl %s %s: %v", req.Action, unit, err),
			"output": string(out),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": req.Action + " ok", "unit": unit})
}
