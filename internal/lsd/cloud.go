package lsd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// defaultSunshineURL is the Sunshine instance the Cloud page talks to. The
// -sunshine-url flag overrides it for development instances.
const defaultSunshineURL = "https://sunshine.rescoot.org"

// cloudService describes one connectivity service lsd can configure.
type cloudService struct {
	Unit       string `json:"unit"`
	ConfigPath string `json:"config-path"`
	// Bootstrap is true when Sunshine can issue this service's config
	// through the scooter bootstrap endpoint. uplink-service configs are
	// downloaded from the Sunshine scooter page and pasted instead.
	Bootstrap bool `json:"bootstrap"`
}

// cloudServices lists the services in the order the UI shows them. Unit
// names and config paths match the deployed unit files (both pass -config).
var cloudServices = []struct {
	name string
	cloudService
}{
	{"radio-gaga", cloudService{Unit: "radio-gaga.service", ConfigPath: "/data/radio-gaga/config.yaml", Bootstrap: true}},
	{"uplink", cloudService{Unit: "librescoot-uplink.service", ConfigPath: "/data/uplink-service/uplink.yaml"}},
}

func cloudServiceByName(name string) (cloudService, bool) {
	for _, c := range cloudServices {
		if c.name == name {
			return c.cloudService, true
		}
	}
	return cloudService{}, false
}

// cloudIdentity is what the scooter can say about itself to Sunshine.
type cloudIdentity struct {
	VIN       string `json:"vin,omitempty"`
	IMEI      string `json:"imei,omitempty"`
	MDBSerial string `json:"mdb-serial,omitempty"`
	DBCSerial string `json:"dbc-serial,omitempty"`
}

// identity reads the hardware identifiers Sunshine matches on. The IMEI
// comes from modem-service's internet hash, the board serials from
// version-service's OCOTP readout, exactly the fields radio-gaga's own
// bootstrap mode submits.
func (s *Server) identity() cloudIdentity {
	var id cloudIdentity
	client := s.getRedis()
	if client == nil {
		return id
	}
	if m, err := client.HGetAll("internet"); err == nil {
		id.IMEI = m["sim-imei"]
	}
	if m, err := client.HGetAll("version:mdb"); err == nil {
		id.MDBSerial = m["serial_number_real"]
	}
	if m, err := client.HGetAll("version:dbc"); err == nil {
		id.DBCSerial = m["serial_number_real"]
	}
	if m, err := client.HGetAll("scooter"); err == nil {
		id.VIN = m["vin"]
	}
	return id
}

// configIdentity is what a service config reveals about its connection.
type configIdentity struct {
	Identifier string `json:"identifier,omitempty"`
	ServerURL  string `json:"server-url,omitempty"`
	// Backend is "sunshine" when the server URL points at the configured
	// Sunshine host, "custom" for any other host, "" when unconfigured.
	Backend string `json:"backend,omitempty"`
}

// parseConfigIdentity pulls the vehicle identifier and the backend URL out
// of a service config. Both flavours carry `identifier` under `scooter:`;
// the connection URL is `mqtt.broker_url` for radio-gaga and
// `uplink.server_url` for uplink-service. A hand-rolled scan is enough for
// these two well-known shapes and avoids a YAML dependency.
func parseConfigIdentity(content, sunshineHost string) configIdentity {
	var ci configIdentity
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "identifier":
			if ci.Identifier == "" && value != "" && !strings.ContainsAny(value, " \t") {
				ci.Identifier = value
			}
		case "broker_url", "server_url":
			if ci.ServerURL == "" && value != "" {
				ci.ServerURL = value
			}
		}
	}
	host := hostOf(ci.ServerURL)
	switch {
	case host == "":
	case sunshineHost != "" && (host == sunshineHost || strings.HasSuffix(host, "."+sunshineHost)):
		ci.Backend = "sunshine"
	default:
		ci.Backend = "custom"
	}
	return ci
}

// hostOf returns the bare host of a URL, tolerating schemes like ssl:// and
// credentials in the authority.
func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// configFlagRe finds the config path in an ExecStart line. Both deployed
// units use Go-style single-dash flags (-config PATH); accept -- as well.
var configFlagRe = regexp.MustCompile(`(?:^|\s)--?config(?:=|\s+)(\S+)`)

// unitConfigPath extracts the config path from a unit's ExecStart so the
// Cloud page edits the file the service actually reads.
func unitConfigPath(unit string) string {
	out, err := exec.Command("systemctl", "cat", unit).Output()
	if err != nil {
		return ""
	}
	return configPathFromUnit(string(out))
}

func configPathFromUnit(unitText string) string {
	for _, line := range strings.Split(unitText, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "ExecStart=") {
			continue
		}
		if m := configFlagRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

// cloudServiceStatus is one row of the Cloud page.
type cloudServiceStatus struct {
	cloudService
	Installed  bool   `json:"installed"`
	Active     string `json:"active,omitempty"`
	Configured bool   `json:"configured"`
	configIdentity
}

// serviceStatuses reports every known connectivity service: installed,
// running, and what its config points at.
func (s *Server) serviceStatuses() map[string]cloudServiceStatus {
	out := map[string]cloudServiceStatus{}
	units, _ := listUnits()
	byUnit := map[string]unitStatus{}
	for _, u := range units {
		byUnit[u.Unit] = u
	}
	sunshineHost := hostOf(s.sunshineURL)

	for _, c := range cloudServices {
		st := cloudServiceStatus{cloudService: c.cloudService}
		if u, ok := byUnit[c.Unit]; ok {
			st.Installed = true
			st.Active = u.Active
			if p := unitConfigPath(c.Unit); p != "" {
				st.ConfigPath = p
			}
		}
		if b, err := os.ReadFile(st.ConfigPath); err == nil {
			st.configIdentity = parseConfigIdentity(string(b), sunshineHost)
			st.Configured = st.Identifier != "" &&
				!strings.Contains(st.Identifier, "VEHICLE-ID") &&
				!strings.Contains(st.Identifier, "changeme")
		}
		out[c.name] = st
	}
	return out
}

// handleCloudStatus answers GET /api/cloud.
func (s *Server) handleCloudStatus(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	id := s.identity()
	services := s.serviceStatuses()
	// A configured service knows the identifier even when the scooter hash
	// does not carry a VIN.
	if id.VIN == "" {
		for _, c := range cloudServices {
			if st := services[c.name]; st.Configured {
				id.VIN = st.Identifier
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"identity":     id,
		"services":     services,
		"sunshine-url": s.sunshineURL,
	})
}

// cloudRequest is the body of both Cloud page POSTs.
type cloudRequest struct {
	// Bootstrap: the bootstrap token the user minted in their Sunshine
	// settings. Same credential radio-gaga's -bootstrap mode takes.
	Token string `json:"token"`

	// Config: which service to configure and the YAML to write.
	Service    string `json:"service"`
	YAML       string `json:"yaml"`
	ConfigPath string `json:"config-path"`
}

// handleCloudBootstrap claims the scooter into the token owner's Sunshine
// account and installs the radio-gaga config Sunshine returns. This is the
// same POST /api/v1/scooters/bootstrap exchange radio-gaga performs itself.
func (s *Server) handleCloudBootstrap(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req cloudRequest
	if !readJSON(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeErr(w, http.StatusBadRequest, "bootstrap token required")
		return
	}
	id := s.identity()
	if id.IMEI == "" && id.MDBSerial == "" {
		writeErr(w, http.StatusServiceUnavailable, "no hardware identifier available yet: neither the modem IMEI nor the MDB serial is in Redis")
		return
	}

	body := map[string]string{
		"imei":             id.IMEI,
		"mdb_serial":       id.MDBSerial,
		"dbc_serial":       id.DBCSerial,
		"platform":         "librescoot",
		"software_version": "lsd " + s.version,
	}
	status, data, err := postJSON(s.sunshineURL+"/api/v1/scooters/bootstrap", token, body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Sunshine unreachable: "+err.Error())
		return
	}
	var resp struct {
		Status     string `json:"status"`
		Message    string `json:"message"`
		ScooterID  int    `json:"scooter_id"`
		ConfigYAML string `json:"config_yaml"`
		Error      struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &resp)
	if status < 200 || status >= 300 {
		msg := firstNonEmpty(resp.Error.Message, resp.Message, strings.TrimSpace(string(data)))
		if len(msg) > 400 {
			msg = msg[:400]
		}
		writeJSON(w, status, map[string]string{"error": fmt.Sprintf("Sunshine: %s", msg)})
		return
	}
	if resp.ConfigYAML == "" {
		writeErr(w, http.StatusBadGateway, "Sunshine answered without a config")
		return
	}

	svc, _ := cloudServiceByName("radio-gaga")
	result := s.installConfig(svc, "radio-gaga", resp.ConfigYAML, "")
	result["status"] = resp.Status
	result["scooter-id"] = resp.ScooterID
	writeJSON(w, http.StatusOK, result)
}

// handleCloudConfig writes a pasted config for a service and restarts it.
func (s *Server) handleCloudConfig(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req cloudRequest
	if !readJSON(w, r, &req) {
		return
	}
	svc, ok := cloudServiceByName(req.Service)
	if !ok {
		writeErr(w, http.StatusBadRequest, `service must be "radio-gaga" or "uplink"`)
		return
	}
	if strings.TrimSpace(req.YAML) == "" {
		writeErr(w, http.StatusBadRequest, "config is empty")
		return
	}
	if parseConfigIdentity(req.YAML, "").Identifier == "" {
		writeErr(w, http.StatusBadRequest, "config has no scooter identifier; this does not look like a service config")
		return
	}
	writeJSON(w, http.StatusOK, s.installConfig(svc, req.Service, req.YAML, req.ConfigPath))
}

// installConfig writes the config atomically, enables the unit so it
// survives reboots, restarts it and reports the resulting state.
func (s *Server) installConfig(svc cloudService, name, content, configPath string) map[string]interface{} {
	if strings.TrimSpace(configPath) == "" {
		configPath = svc.ConfigPath
		if p := unitConfigPath(svc.Unit); p != "" {
			configPath = p
		}
	}
	result := map[string]interface{}{
		"service":     name,
		"unit":        svc.Unit,
		"config-path": configPath,
		"identifier":  parseConfigIdentity(content, hostOf(s.sunshineURL)).Identifier,
	}
	if err := writeFileAtomic(configPath, strings.NewReader(content), 0o600); err != nil {
		result["error"] = "write config: " + err.Error()
		return result
	}
	if out, err := exec.Command("systemctl", "enable", svc.Unit).CombinedOutput(); err != nil {
		result["enable-error"] = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "restart", svc.Unit).CombinedOutput(); err != nil {
		result["restart-error"] = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	invalidateUnitCache()
	if rows, err := listUnits(); err == nil {
		for _, u := range rows {
			if u.Unit == svc.Unit {
				result["active"] = u.Active
				break
			}
		}
	}
	return result
}

// postJSON posts a JSON body with a bearer token and returns the response.
func postJSON(endpoint, token string, body interface{}) (int, []byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
