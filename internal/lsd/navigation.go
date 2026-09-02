package lsd

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/redis"
)

// Saved locations live in the settings hash as
// dashboard.saved-locations.<id>.<field>, the layout the dashboard and lsc
// share. A change is announced with one publish of the id prefix.
const locationsPrefix = "dashboard.saved-locations"

var locationKeyRe = regexp.MustCompile(`^dashboard\.saved-locations\.(\d+)\.(latitude|longitude|label|created-at|last-used-at)$`)

type savedLocation struct {
	ID         int     `json:"id"`
	Label      string  `json:"label"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	CreatedAt  string  `json:"created-at,omitempty"`
	LastUsedAt string  `json:"last-used-at,omitempty"`
}

func loadLocations(client *redis.Client) ([]savedLocation, error) {
	settings, err := client.HGetAll("settings")
	if err != nil {
		return nil, err
	}
	byID := map[int]map[string]string{}
	for k, v := range settings {
		m := locationKeyRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		if byID[id] == nil {
			byID[id] = map[string]string{}
		}
		byID[id][m[2]] = v
	}
	out := []savedLocation{}
	for id, f := range byID {
		lat, err1 := strconv.ParseFloat(f["latitude"], 64)
		lon, err2 := strconv.ParseFloat(f["longitude"], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, savedLocation{ID: id, Label: f["label"], Latitude: lat, Longitude: lon, CreatedAt: f["created-at"], LastUsedAt: f["last-used-at"]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func validCoords(lat, lon float64) error {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fmt.Errorf("latitude must be within -90..90 and longitude within -180..180")
	}
	if lat == 0 && lon == 0 {
		return fmt.Errorf("0, 0 is not a destination")
	}
	return nil
}

// handleNavigation answers GET /api/navigation: the current destination and
// the saved locations.
func (s *Server) handleNavigation(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		client := s.getRedis()
		if client == nil {
			writeErr(w, http.StatusServiceUnavailable, "redis not connected")
			return
		}
		dest, _ := client.HGetAll("navigation")
		locs, err := loadLocations(client)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"destination": dest, "locations": locs})
	case http.MethodPost:
		s.setDestination(w, r)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// setDestination writes the navigation hash the way the dashboard and
// radio-gaga do: lat/lon to six places, the legacy "lat,lon" destination,
// an RFC 3339 timestamp, then one notification per field with destination
// last, since that is the field the dashboard acts on. Clearing sets every
// field to the empty string so watchers see it.
func (s *Server) setDestination(w http.ResponseWriter, r *http.Request) {
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	var req struct {
		Clear      bool    `json:"clear"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Address    string  `json:"address"`
		LocationID *int    `json:"location-id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	fields := map[string]string{}
	if req.Clear {
		for _, f := range []string{"latitude", "longitude", "address", "timestamp", "destination"} {
			fields[f] = ""
		}
	} else {
		if err := validCoords(req.Latitude, req.Longitude); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		lat, lon := fmt.Sprintf("%.6f", req.Latitude), fmt.Sprintf("%.6f", req.Longitude)
		fields["latitude"] = lat
		fields["longitude"] = lon
		fields["address"] = strings.TrimSpace(req.Address)
		fields["timestamp"] = time.Now().UTC().Format(time.RFC3339)
		fields["destination"] = lat + "," + lon
	}
	for _, f := range []string{"latitude", "longitude", "address", "timestamp", "destination"} {
		if err := client.HSet("navigation", f, fields[f]); err != nil {
			writeErr(w, http.StatusBadGateway, "write navigation: "+err.Error())
			return
		}
	}
	for _, f := range []string{"latitude", "longitude", "address", "timestamp", "destination"} {
		_ = client.Publish(r.Context(), "navigation", f)
	}
	if req.LocationID != nil && !req.Clear {
		key := fmt.Sprintf("%s.%d.last-used-at", locationsPrefix, *req.LocationID)
		if err := client.HSet("settings", key, time.Now().UTC().Format(time.RFC3339)); err == nil {
			_ = client.Publish(r.Context(), "settings", fmt.Sprintf("%s.%d", locationsPrefix, *req.LocationID))
		}
	}
	status := "set"
	if req.Clear {
		status = "cleared"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": status, "destination": fields})
}

// handleLocations implements PUT (create or update) and DELETE on saved
// locations.
func (s *Server) handleLocations(w http.ResponseWriter, r *http.Request) {
	client := s.getRedis()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis not connected")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req struct {
			ID        *int    `json:"id"`
			Label     string  `json:"label"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		req.Label = strings.TrimSpace(req.Label)
		if req.Label == "" || len(req.Label) > 120 {
			writeErr(w, http.StatusBadRequest, "a label of up to 120 characters is required")
			return
		}
		if err := validCoords(req.Latitude, req.Longitude); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		locs, err := loadLocations(client)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		id := -1
		created := now
		if req.ID != nil {
			for _, l := range locs {
				if l.ID == *req.ID {
					id = l.ID
					if l.CreatedAt != "" {
						created = l.CreatedAt
					}
				}
			}
			if id < 0 {
				writeErr(w, http.StatusNotFound, "no such location")
				return
			}
		} else {
			used := map[int]bool{}
			for _, l := range locs {
				used[l.ID] = true
			}
			for id = 0; used[id]; id++ {
			}
		}
		fields := map[string]string{
			"label":        req.Label,
			"latitude":     fmt.Sprintf("%.6f", req.Latitude),
			"longitude":    fmt.Sprintf("%.6f", req.Longitude),
			"created-at":   created,
			"last-used-at": now,
		}
		for f, v := range fields {
			if err := client.HSet("settings", fmt.Sprintf("%s.%d.%s", locationsPrefix, id, f), v); err != nil {
				writeErr(w, http.StatusBadGateway, "write settings: "+err.Error())
				return
			}
		}
		_ = client.Publish(r.Context(), "settings", fmt.Sprintf("%s.%d", locationsPrefix, id))
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "saved", "id": id})
	case http.MethodDelete:
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || id < 0 {
			writeErr(w, http.StatusBadRequest, "id required")
			return
		}
		for _, f := range []string{"latitude", "longitude", "label", "created-at", "last-used-at"} {
			if err := client.HDel("settings", fmt.Sprintf("%s.%d.%s", locationsPrefix, id, f)); err != nil {
				writeErr(w, http.StatusBadGateway, "delete: "+err.Error())
				return
			}
		}
		_ = client.Publish(r.Context(), "settings", fmt.Sprintf("%s.%d", locationsPrefix, id))
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
