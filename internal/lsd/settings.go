package lsd

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/schema"
)

// handleSettingsSchema serves the raw settings:schema document.
func (s *Server) handleSettingsSchema(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	raw, err := client.Get("settings:schema")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "read settings:schema: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(raw))
}

// handleSettings answers GET /api/settings with current values.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	values, err := client.HGetAll("settings")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "read settings: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"values": values})
}

// settingsUpdate is the body of PUT /api/settings/set.
type settingsUpdate struct {
	// Values maps setting keys to their new value. An empty string deletes
	// the key so the schema default shows through again.
	Values map[string]string `json:"values"`
}

// handleSettingsSet applies validated setting writes: hash first, then the
// change notification, exactly the contract settings-service documents.
func (s *Server) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPut) {
		return
	}
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	var req settingsUpdate
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Values) == 0 {
		writeErr(w, http.StatusBadRequest, "no values given")
		return
	}

	// The schema may have changed since boot; refresh once per write batch
	// so validation matches what settings-service currently publishes.
	if err := s.reloadSchema(); err != nil && s.getSchema() == nil {
		writeErr(w, http.StatusBadGateway, "settings schema unavailable: "+err.Error())
		return
	}

	applied := map[string]string{}
	failures := map[string]string{}
	for key, value := range req.Values {
		if err := s.validateSetting(key, value); err != nil {
			failures[key] = err.Error()
			continue
		}
		var err error
		if value == "" {
			err = client.HDel("settings", key)
		} else {
			err = client.HSet("settings", key, value)
		}
		if err != nil {
			failures[key] = err.Error()
			continue
		}
		applied[key] = value
	}

	// Publish after the hash writes so subscribers always read fresh values.
	for key := range applied {
		_ = client.Publish(r.Context(), "settings", key)
	}

	status := http.StatusOK
	if len(failures) > 0 {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]interface{}{
		"applied":  applied,
		"failures": failures,
	})
}

// validateSetting checks one key/value pair against the schema.
func (s *Server) validateSetting(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("empty key")
	}
	if strings.ContainsAny(key, " \t\n") {
		return fmt.Errorf("key must not contain whitespace")
	}
	sch := s.getSchema()
	if sch == nil {
		// Without schema metadata, accept plain keys: lsc's settings set
		// does the same, and settings-service persists whatever it sees.
		return nil
	}
	setting, ok := sch.Get(key)
	if !ok {
		return fmt.Errorf("unknown setting")
	}
	if setting.ReadOnly {
		return fmt.Errorf("setting is read-only")
	}
	if value == "" {
		return nil // reset to default
	}
	switch setting.Type {
	case "bool":
		if value != "true" && value != "false" {
			return fmt.Errorf("expected true or false")
		}
	case "int":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("expected an integer")
		}
		return checkRange(setting, value)
	case "float":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("expected a number")
		}
		return checkRange(setting, value)
	case "enum":
		for _, v := range setting.Values {
			if v.Value == value {
				return nil
			}
		}
		return fmt.Errorf("must be one of: %s", strings.Join(setting.PossibleValues(), ", "))
	case "duration":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("expected a duration like 30s, 5m or 1h")
		}
	case "url":
		if !strings.Contains(value, "://") {
			return fmt.Errorf("expected a URL including scheme")
		}
	}
	// Strings and pattern-typed keys are validated by settings-service,
	// which has the final say anyway.
	return nil
}

// checkRange enforces min/max on numeric settings.
func checkRange(setting schema.Setting, value string) error {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	if setting.Min != nil && f < *setting.Min {
		return fmt.Errorf("must be at least %v", *setting.Min)
	}
	if setting.Max != nil && f > *setting.Max {
		return fmt.Errorf("must be at most %v", *setting.Max)
	}
	return nil
}
