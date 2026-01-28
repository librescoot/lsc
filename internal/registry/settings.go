package registry

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SettingType defines the data type of a setting value
type SettingType string

const (
	TypeBool     SettingType = "bool"
	TypeInt      SettingType = "int"
	TypeFloat    SettingType = "float"
	TypeString   SettingType = "string"
	TypeEnum     SettingType = "enum"
	TypeURL      SettingType = "url"
	TypeDuration SettingType = "duration"
)

// Setting describes a LibreScoot setting with full metadata
type Setting struct {
	Key            string      // Redis key (dot notation)
	Service        string      // Service that owns/uses this setting
	Type           SettingType // Data type
	Description    string      // Human-readable description
	PossibleValues []string    // For enums, or documented valid values
	DefaultValue   string      // Default value (as string)
	Unit           string      // Unit of measurement (e.g., "seconds", "km/h", "hours")
	MinValue       *float64    // For numeric types, minimum allowed value
	MaxValue       *float64    // For numeric types, maximum allowed value
	Example        string      // Example value for documentation
	Pattern        string      // Special pattern indicator (e.g., "indexed" for saved-locations)
}

// Settings is the registry of all known LibreScoot settings
var Settings = []Setting{
	// Alarm settings (alarm-service)
	{
		Key:            "alarm.enabled",
		Service:        "alarm-service",
		Type:           TypeBool,
		Description:    "Enable or disable the alarm system",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},
	{
		Key:            "alarm.honk",
		Service:        "alarm-service",
		Type:           TypeBool,
		Description:    "Enable horn during alarm trigger",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},
	{
		Key:          "alarm.duration",
		Service:      "alarm-service",
		Type:         TypeInt,
		Description:  "Duration in seconds for alarm sound",
		DefaultValue: "60",
		Unit:         "seconds",
		MinValue:     ptr(0.0),
		MaxValue:     ptr(300.0),
		Example:      "60",
	},
	{
		Key:            "alarm.seatbox-trigger",
		Service:        "alarm-service",
		Type:           TypeBool,
		Description:    "Trigger alarm on unauthorized seatbox opening",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "true",
		Example:        "true",
	},
	{
		Key:            "alarm.hairtrigger",
		Service:        "alarm-service",
		Type:           TypeBool,
		Description:    "Enable hair trigger mode (immediate short alarm on first motion)",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},
	{
		Key:          "alarm.hairtrigger-duration",
		Service:      "alarm-service",
		Type:         TypeInt,
		Description:  "Duration in seconds for hair trigger alarm",
		DefaultValue: "3",
		Unit:         "seconds",
		MinValue:     ptr(1.0),
		MaxValue:     ptr(30.0),
		Example:      "3",
	},

	// Scooter settings (battery-service)
	{
		Key:            "scooter.battery-ignores-seatbox",
		Service:        "battery-service",
		Type:           TypeBool,
		Description:    "Ignore seatbox open and always keep batteries active",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "false",
	},
	{
		Key:            "scooter.dual-battery",
		Service:        "battery-service",
		Type:           TypeBool,
		Description:    "Enable dual battery mode (battery 1 active instead of idle)",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},

	// Power management settings (pm-service)
	{
		Key:          "hibernation-timer",
		Service:      "pm-service",
		Type:         TypeInt,
		Description:  "Hibernation timeout in seconds",
		DefaultValue: "432000",
		Unit:         "seconds",
		MinValue:     ptr(0.0),
		Example:      "432000",
	},

	// Vehicle settings (vehicle-service)
	{
		Key:          "scooter.auto-standby-seconds",
		Service:      "vehicle-service",
		Type:         TypeInt,
		Description:  "Auto-lock timeout when parked in seconds (0=disabled)",
		DefaultValue: "0",
		Unit:         "seconds",
		MinValue:     ptr(0.0),
		Example:      "300",
	},
	{
		Key:            "scooter.brake-hibernation",
		Service:        "vehicle-service",
		Type:           TypeEnum,
		Description:    "Enable brake lever hibernation",
		PossibleValues: []string{"enabled", "disabled"},
		DefaultValue:   "enabled",
		Example:        "enabled",
	},
	{
		Key:            "scooter.enable-horn",
		Service:        "vehicle-service",
		Type:           TypeEnum,
		Description:    "Horn enable mode",
		PossibleValues: []string{"true", "false", "in-drive"},
		DefaultValue:   "true",
		Example:        "in-drive",
	},

	// Update service settings - MDB (update-service)
	{
		Key:            "updates.mdb.method",
		Service:        "update-service",
		Type:           TypeEnum,
		Description:    "Update method for MDB",
		PossibleValues: []string{"delta", "full"},
		DefaultValue:   "full",
		Example:        "delta",
	},
	{
		Key:            "updates.mdb.channel",
		Service:        "update-service",
		Type:           TypeEnum,
		Description:    "Release channel for MDB",
		PossibleValues: []string{"stable", "testing", "nightly"},
		DefaultValue:   "nightly",
		Example:        "stable",
	},
	{
		Key:          "updates.mdb.check-interval",
		Service:      "update-service",
		Type:         TypeDuration,
		Description:  "Time between update checks for MDB (0 to disable)",
		DefaultValue: "6h",
		MinValue:     ptr(0.0),
		Example:      "12h",
	},
	{
		Key:          "updates.mdb.last-check-time",
		Service:      "update-service",
		Type:         TypeString,
		Description:  "Last time MDB checked for updates (ISO8601 timestamp)",
		DefaultValue: "",
		Example:      "2025-01-15T10:30:00Z",
	},
	{
		Key:          "updates.mdb.github-releases-url",
		Service:      "update-service",
		Type:         TypeURL,
		Description:  "GitHub Releases API endpoint for MDB",
		DefaultValue: "https://api.github.com/repos/librescoot/librescoot/releases",
		Example:      "https://api.github.com/repos/librescoot/librescoot/releases",
	},
	{
		Key:            "updates.mdb.dry-run",
		Service:        "update-service",
		Type:           TypeBool,
		Description:    "Enable dry-run mode for MDB updates (no reboot)",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},

	// Update service settings - DBC (update-service)
	{
		Key:            "updates.dbc.method",
		Service:        "update-service",
		Type:           TypeEnum,
		Description:    "Update method for DBC",
		PossibleValues: []string{"delta", "full"},
		DefaultValue:   "full",
		Example:        "delta",
	},
	{
		Key:            "updates.dbc.channel",
		Service:        "update-service",
		Type:           TypeEnum,
		Description:    "Release channel for DBC",
		PossibleValues: []string{"stable", "testing", "nightly"},
		DefaultValue:   "nightly",
		Example:        "stable",
	},
	{
		Key:          "updates.dbc.check-interval",
		Service:      "update-service",
		Type:         TypeDuration,
		Description:  "Time between update checks for DBC (0 to disable)",
		DefaultValue: "6h",
		MinValue:     ptr(0.0),
		Example:      "12h",
	},
	{
		Key:          "updates.dbc.last-check-time",
		Service:      "update-service",
		Type:         TypeString,
		Description:  "Last time DBC checked for updates (ISO8601 timestamp)",
		DefaultValue: "",
		Example:      "2025-01-15T10:30:00Z",
	},
	{
		Key:          "updates.dbc.github-releases-url",
		Service:      "update-service",
		Type:         TypeURL,
		Description:  "GitHub Releases API endpoint for DBC",
		DefaultValue: "https://api.github.com/repos/librescoot/librescoot/releases",
		Example:      "https://api.github.com/repos/librescoot/librescoot/releases",
	},
	{
		Key:            "updates.dbc.dry-run",
		Service:        "update-service",
		Type:           TypeBool,
		Description:    "Enable dry-run mode for DBC updates (no reboot)",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},

	// Network settings (modem-service)
	{
		Key:          "cellular.apn",
		Service:      "modem-service",
		Type:         TypeString,
		Description:  "Cellular APN string",
		DefaultValue: "",
		Example:      "internet",
	},

	// Dashboard settings (scootui)
	{
		Key:            "dashboard.show-raw-speed",
		Service:        "scootui",
		Type:           TypeBool,
		Description:    "Show raw uncorrected speed from ECU",
		PossibleValues: []string{"true", "false"},
		DefaultValue:   "false",
		Example:        "true",
	},
	{
		Key:            "dashboard.show-clock",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Clock visibility",
		PossibleValues: []string{"always", "never"},
		DefaultValue:   "always",
		Example:        "always",
	},
	{
		Key:            "dashboard.show-gps",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "GPS indicator visibility",
		PossibleValues: []string{"always", "active-or-error", "error", "never"},
		DefaultValue:   "error",
		Example:        "active-or-error",
	},
	{
		Key:            "dashboard.show-bluetooth",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Bluetooth indicator visibility",
		PossibleValues: []string{"always", "active-or-error", "error", "never"},
		DefaultValue:   "active-or-error",
		Example:        "always",
	},
	{
		Key:            "dashboard.show-cloud",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Cloud indicator visibility",
		PossibleValues: []string{"always", "active-or-error", "error", "never"},
		DefaultValue:   "error",
		Example:        "never",
	},
	{
		Key:            "dashboard.show-internet",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Internet indicator visibility",
		PossibleValues: []string{"always", "active-or-error", "error", "never"},
		DefaultValue:   "always",
		Example:        "always",
	},
	{
		Key:            "dashboard.battery-display-mode",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Battery display mode",
		PossibleValues: []string{"percentage", "range"},
		DefaultValue:   "percentage",
		Example:        "range",
	},
	{
		Key:            "dashboard.map.type",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Map tile source",
		PossibleValues: []string{"online", "offline"},
		DefaultValue:   "offline",
		Example:        "online",
	},
	{
		Key:            "dashboard.map.render-mode",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Map rendering mode",
		PossibleValues: []string{"vector", "raster"},
		DefaultValue:   "raster",
		Example:        "vector",
	},
	{
		Key:            "dashboard.theme",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "UI theme",
		PossibleValues: []string{"light", "dark", "auto"},
		DefaultValue:   "dark",
		Example:        "auto",
	},
	{
		Key:            "dashboard.mode",
		Service:        "scootui",
		Type:           TypeEnum,
		Description:    "Default screen mode",
		PossibleValues: []string{"speedometer", "navigation"},
		DefaultValue:   "speedometer",
		Example:        "navigation",
	},
	{
		Key:          "dashboard.valhalla-url",
		Service:      "scootui",
		Type:         TypeURL,
		Description:  "Valhalla routing service endpoint",
		DefaultValue: "http://localhost:8002/",
		Example:      "http://localhost:8002/",
	},

	// Saved locations (scootui) - Pattern-based settings
	// Note: These are examples. Actual indices 0-N are dynamically recognized.
	{
		Key:          "dashboard.saved-locations.0.created-at",
		Service:      "scootui",
		Type:         TypeString,
		Description:  "Creation timestamp (example: index 0)",
		DefaultValue: "",
		Example:      "2025-01-15T10:30:00Z",
		Pattern:      "indexed",
	},
	{
		Key:          "dashboard.saved-locations.0.label",
		Service:      "scootui",
		Type:         TypeString,
		Description:  "Label (example: index 0)",
		DefaultValue: "",
		Example:      "Home",
		Pattern:      "indexed",
	},
	{
		Key:          "dashboard.saved-locations.0.last-used-at",
		Service:      "scootui",
		Type:         TypeString,
		Description:  "Last used timestamp (example: index 0)",
		DefaultValue: "",
		Example:      "2025-01-15T12:00:00Z",
		Pattern:      "indexed",
	},
	{
		Key:          "dashboard.saved-locations.0.latitude",
		Service:      "scootui",
		Type:         TypeFloat,
		Description:  "Latitude (example: index 0)",
		DefaultValue: "",
		MinValue:     ptr(-90.0),
		MaxValue:     ptr(90.0),
		Example:      "52.5200",
		Pattern:      "indexed",
	},
	{
		Key:          "dashboard.saved-locations.0.longitude",
		Service:      "scootui",
		Type:         TypeFloat,
		Description:  "Longitude (example: index 0)",
		DefaultValue: "",
		MinValue:     ptr(-180.0),
		MaxValue:     ptr(180.0),
		Example:      "13.4050",
		Pattern:      "indexed",
	},
}

// Helper function to create a pointer to a float64 (for min/max values)
func ptr(f float64) *float64 {
	return &f
}

// GetSetting retrieves a setting by key
func GetSetting(key string) (*Setting, bool) {
	for i := range Settings {
		if Settings[i].Key == key {
			return &Settings[i], true
		}
	}
	return nil, false
}

// GetSettingsByService retrieves all settings for a specific service
func GetSettingsByService(service string) []Setting {
	var result []Setting
	for _, s := range Settings {
		if s.Service == service {
			result = append(result, s)
		}
	}
	return result
}

// GetServices returns a list of unique service names
func GetServices() []string {
	seen := make(map[string]bool)
	var services []string
	for _, s := range Settings {
		if !seen[s.Service] {
			services = append(services, s.Service)
			seen[s.Service] = true
		}
	}
	return services
}

// ValidateValue validates a value against the setting's constraints
func ValidateValue(key, value string) error {
	setting, found := GetSetting(key)
	if !found {
		// Unknown settings are allowed (forward compatibility)
		return nil
	}

	if value == "" {
		// Empty values are allowed (means "not set")
		return nil
	}

	switch setting.Type {
	case TypeBool:
		if value != "true" && value != "false" {
			return fmt.Errorf("must be 'true' or 'false'")
		}

	case TypeInt:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if setting.MinValue != nil && float64(intVal) < *setting.MinValue {
			return fmt.Errorf("must be >= %.0f", *setting.MinValue)
		}
		if setting.MaxValue != nil && float64(intVal) > *setting.MaxValue {
			return fmt.Errorf("must be <= %.0f", *setting.MaxValue)
		}

	case TypeFloat:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		if setting.MinValue != nil && floatVal < *setting.MinValue {
			return fmt.Errorf("must be >= %.2f", *setting.MinValue)
		}
		if setting.MaxValue != nil && floatVal > *setting.MaxValue {
			return fmt.Errorf("must be <= %.2f", *setting.MaxValue)
		}

	case TypeEnum:
		if len(setting.PossibleValues) > 0 {
			valid := false
			for _, pv := range setting.PossibleValues {
				if value == pv {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("must be one of: %s", strings.Join(setting.PossibleValues, ", "))
			}
		}

	case TypeURL:
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("must be a valid URL (http:// or https://)")
		}

	case TypeDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("must be a valid duration (e.g., '1h', '30m', '1h30m')")
		}
		// Additional constraints can be checked here if MinValue/MaxValue are set
		if setting.MinValue != nil || setting.MaxValue != nil {
			d, _ := time.ParseDuration(value)
			seconds := d.Seconds()
			if setting.MinValue != nil && seconds < *setting.MinValue {
				return fmt.Errorf("must be >= %v", time.Duration(*setting.MinValue*float64(time.Second)))
			}
			if setting.MaxValue != nil && seconds > *setting.MaxValue {
				return fmt.Errorf("must be <= %v", time.Duration(*setting.MaxValue*float64(time.Second)))
			}
		}
	}

	return nil
}

// IsKnownSetting checks if a key is in the registry
func IsKnownSetting(key string) bool {
	_, found := GetSetting(key)
	if found {
		return true
	}

	// Check if it matches the saved-locations pattern
	if strings.HasPrefix(key, "dashboard.saved-locations.") {
		return true
	}

	return false
}
