package locations

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool
)

const (
	locationsKeyPrefix = "dashboard.saved-locations"
)

// SavedLocation represents a saved location
type SavedLocation struct {
	ID         int
	Latitude   float64
	Longitude  float64
	Label      string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

var LocationsCmd = &cobra.Command{
	Use:     "locations",
	Aliases: []string{"loc"},
	Short:   "Manage saved locations",
	Long:    `Manage saved locations for navigation.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to list when called without subcommand
		listCmd.Run(cmd, args)
	},
}

// SetRedisClient sets the Redis client for all locations commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for all locations commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// loadAllLocations discovers and loads all saved locations from Redis
func loadAllLocations() ([]SavedLocation, error) {
	// Get all keys matching the pattern
	client := RedisClient.GetClient()
	ctx := context.Background()
	keys, err := client.Keys(ctx, locationsKeyPrefix+".*").Result()
	if err != nil {
		return nil, err
	}

	// Extract unique IDs from keys
	idMap := make(map[int]bool)
	re := regexp.MustCompile(`^dashboard\.saved-locations\.(\d+)\.`)
	for _, key := range keys {
		matches := re.FindStringSubmatch(key)
		if len(matches) >= 2 {
			if id, err := strconv.Atoi(matches[1]); err == nil {
				idMap[id] = true
			}
		}
	}

	// Load each location
	locations := []SavedLocation{}
	for id := range idMap {
		loc, err := loadLocation(id)
		if err == nil && loc != nil {
			locations = append(locations, *loc)
		}
	}

	// Sort by last used (most recent first)
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].LastUsedAt.After(locations[j].LastUsedAt)
	})

	return locations, nil
}

// loadLocation loads a single location by ID
func loadLocation(id int) (*SavedLocation, error) {
	fields := []string{"latitude", "longitude", "label", "created-at", "last-used-at"}
	data := make(map[string]string)

	for _, field := range fields {
		key := fmt.Sprintf("%s.%d.%s", locationsKeyPrefix, id, field)
		value, err := RedisClient.HGet("settings", key)
		if err != nil {
			// Field doesn't exist, skip this location
			return nil, err
		}
		if value != "" {
			data[field] = value
		}
	}

	// If we don't have all required fields, skip
	if len(data) < 3 {
		return nil, fmt.Errorf("incomplete location data")
	}

	lat, err := strconv.ParseFloat(data["latitude"], 64)
	if err != nil {
		return nil, err
	}

	lon, err := strconv.ParseFloat(data["longitude"], 64)
	if err != nil {
		return nil, err
	}

	createdAt, _ := time.Parse(time.RFC3339, data["created-at"])
	lastUsedAt, _ := time.Parse(time.RFC3339, data["last-used-at"])

	return &SavedLocation{
		ID:         id,
		Latitude:   lat,
		Longitude:  lon,
		Label:      data["label"],
		CreatedAt:  createdAt,
		LastUsedAt: lastUsedAt,
	}, nil
}

// saveLocation saves or updates a location
func saveLocation(loc SavedLocation) error {
	fields := map[string]string{
		"latitude":     fmt.Sprintf("%.6f", loc.Latitude),
		"longitude":    fmt.Sprintf("%.6f", loc.Longitude),
		"label":        loc.Label,
		"created-at":   loc.CreatedAt.Format(time.RFC3339),
		"last-used-at": loc.LastUsedAt.Format(time.RFC3339),
	}

	for field, value := range fields {
		key := fmt.Sprintf("%s.%d.%s", locationsKeyPrefix, loc.ID, field)
		if err := RedisClient.HSet("settings", key, value); err != nil {
			return err
		}
	}

	// Publish notification
	client := RedisClient.GetClient()
	ctx := context.Background()
	return client.Publish(ctx, "settings", fmt.Sprintf("%s.%d", locationsKeyPrefix, loc.ID)).Err()
}

// deleteLocation deletes a location by ID
func deleteLocation(id int) error {
	fields := []string{"latitude", "longitude", "label", "created-at", "last-used-at"}
	client := RedisClient.GetClient()
	ctx := context.Background()

	for _, field := range fields {
		key := fmt.Sprintf("%s.%d.%s", locationsKeyPrefix, id, field)
		if err := client.HDel(ctx, "settings", key).Err(); err != nil {
			return err
		}
	}

	// Publish notification
	return client.Publish(ctx, "settings", fmt.Sprintf("%s.%d", locationsKeyPrefix, id)).Err()
}

// findNextAvailableID finds the next available ID slot
func findNextAvailableID() (int, error) {
	locations, err := loadAllLocations()
	if err != nil {
		return 0, err
	}

	// Build set of used IDs
	usedIDs := make(map[int]bool)
	for _, loc := range locations {
		usedIDs[loc.ID] = true
	}

	// Find first available ID starting from 0
	for i := 0; ; i++ {
		if !usedIDs[i] {
			return i, nil
		}
	}
}

// formatRelativeTime formats a time as relative to now
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	duration := time.Since(t)
	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else if duration < 7*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	} else {
		return t.Format("2006-01-02")
	}
}

// validateCoordinates validates latitude and longitude
func validateCoordinates(lat, lon float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	return nil
}

// parseFieldValuePairs parses field-value pairs from arguments
func parseFieldValuePairs(args []string) (map[string]string, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("fields and values must be provided in pairs")
	}

	updates := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		field := strings.ToLower(args[i])
		value := args[i+1]

		// Validate field names
		switch field {
		case "label":
			updates["label"] = value
		case "lat", "latitude":
			updates["latitude"] = value
		case "lon", "lng", "longitude":
			updates["longitude"] = value
		default:
			return nil, fmt.Errorf("invalid field: %s (valid: label, lat, lon)", field)
		}
	}

	return updates, nil
}

func init() {
}
