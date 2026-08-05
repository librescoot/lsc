package service

import (
	"os/exec"
	"strings"
	"sync"

	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var RedisClient *redis.Client
var JSONOutput *bool

// SetRedisClient allows the parent command to inject the Redis client
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput allows the parent command to inject the JSON output flag
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// serviceNameMap maps shorthand names to full service names
var serviceNameMap = map[string]string{
	"vehicle":    "librescoot-vehicle",
	"battery":    "librescoot-battery",
	"ecu":        "librescoot-ecu",
	"modem":      "librescoot-modem",
	"alarm":      "librescoot-alarm",
	"settings":   "librescoot-settings",
	"keycard":    "librescoot-keycard",
	"boot-led":   "librescoot-boot-led",
	"bluetooth":  "librescoot-bluetooth",
	"ums":        "librescoot-ums",
	"brightness": "librescoot-brightness",
	"onboot":     "librescoot-onboot",
	"backlight":  "dbc-backlight",
	"pm":         "librescoot-pm",
	"update":     "librescoot-update",
	"version":    "librescoot-version",
	"netconfig":  "librescoot-netconfig",
}

var (
	datastoreOnce sync.Once
	datastoreName string
)

// datastoreUnit returns the unit name of the key-value store. Librescoot 1.2
// replaced Redis with Valkey (same protocol, same port, different unit);
// earlier images still ship redis.service. Ask systemd which one exists rather
// than baking in a version assumption, so one binary serves both.
func datastoreUnit() string {
	datastoreOnce.Do(func() {
		datastoreName = "redis"
		out, err := exec.Command("systemctl", "show", "valkey.service",
			"--property=LoadState", "--value").Output()
		if err == nil && strings.TrimSpace(string(out)) == "loaded" {
			datastoreName = "valkey"
		}
	})
	return datastoreName
}

// resolveServiceName maps shorthand names to full service names
func resolveServiceName(name string) string {
	// Remove .service suffix if present for mapping
	baseName := strings.TrimSuffix(name, ".service")

	// Either datastore name reaches whichever unit this image actually has,
	// so an old habit or an old script keeps working across the 1.2 switch.
	if baseName == "redis" || baseName == "valkey" {
		return datastoreUnit()
	}

	// Check if there's a mapping
	if fullName, ok := serviceNameMap[baseName]; ok {
		return fullName
	}

	// Return original name if no mapping found
	return baseName
}

// ensureServiceSuffix adds .service suffix if not present
func ensureServiceSuffix(name string) string {
	// First resolve the service name
	resolved := resolveServiceName(name)

	// Then add .service suffix if not present
	if strings.HasSuffix(resolved, ".service") {
		return resolved
	}
	return resolved + ".service"
}

// ServiceCmd represents the service command
var ServiceCmd = &cobra.Command{
	Use:     "service",
	Short:   "Manage systemd services",
	Long:    `Start, stop, restart, enable, disable, and view logs of Librescoot systemd services.`,
	Aliases: []string{"svc"},
}
