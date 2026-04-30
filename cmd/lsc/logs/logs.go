package logs

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool
	ToolVersion = "dev"

	// Flags
	logsSince    string
	logsUntil    string
	logsOutput   string
	logsPriority string
)

// Service name mappings
var serviceMap = map[string]string{
	"vehicle":    "librescoot-vehicle.service",
	"battery":    "librescoot-battery.service",
	"ecu":        "librescoot-ecu.service",
	"motor":      "librescoot-ecu.service", // alias
	"modem":      "librescoot-modem.service",
	"pm":         "librescoot-pm.service",
	"power":      "librescoot-pm.service", // alias
	"update":     "librescoot-update.service",
	"settings":   "librescoot-settings.service",
	"keycard":    "librescoot-keycard.service",
	"bluetooth":  "librescoot-bluetooth.service",
	"ble":        "librescoot-bluetooth.service", // alias
	"ums":        "librescoot-ums.service",
	"radio-gaga": "radio-gaga.service",
	"uplink":     "radio-gaga.service", // alias
}

// Redis keys to snapshot
var redisKeys = []string{
	"settings", "vehicle", "gps", "gps:filtered", "gps:raw",
	"battery:0", "battery:1", "aux-battery", "cb-battery",
	"engine-ecu", "power-manager", "modem", "internet",
	"alarm", "ble", "system", "dashboard", "ota",
	"power-mux", "version:mdb", "version:dbc",
}

var LogsCmd = &cobra.Command{
	Use:   "logs [services...]",
	Short: "Extract service logs and system state",
	Long: `Extract systemd service logs and Redis snapshots for debugging and analysis.

Available services:
  vehicle, battery, ecu/motor, modem, pm/power, update, settings,
  keycard, bluetooth/ble, ums, radio-gaga/uplink, all (default)

The command will:
  1. Extract journalctl logs for specified services
  2. Collect kernel ring buffer (dmesg)
  3. Capture current Redis state snapshots
  4. Generate metadata file
  5. Write a compressed .tar.gz archive to the output directory

Examples:
  lsc logs                      # Extract all services (default)
  lsc logs vehicle --since 24h
  lsc logs all --since 1h --output /data/debug-session
  lsc logs battery ecu --since "2025-10-25 10:00" --until "2025-10-25 12:00"
  lsc logs all --since 1d --priority err`,
	Run: runLogsExtract,
}

// SetRedisClient sets the Redis client for logs commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for logs commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// SetVersion sets the tool version string embedded in bundle metadata.
func SetVersion(v string) {
	if v != "" {
		ToolVersion = v
	}
}

func runLogsExtract(cmd *cobra.Command, args []string) {
	// Resolve the destination directory that will hold the final tarball,
	// and a per-run staging directory beneath it for the unpacked tree.
	destDir := logsOutput
	if destDir == "" {
		destDir = "/data/log-bundles"
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to create destination directory: %v\n"), err)
		return
	}

	timestamp := time.Now().Format("2006-01-02-15-04")
	bundleName := "logs-" + timestamp
	outputDir := filepath.Join(destDir, ".staging-"+timestamp)
	tarballPath := filepath.Join(destDir, bundleName+".tar.gz")
	defer os.RemoveAll(outputDir)

	// Default to "all" if no services specified
	if len(args) == 0 {
		args = []string{"all"}
	}

	// Determine which services to extract
	var services []string
	if args[0] == "all" {
		for _, svc := range serviceMap {
			// De-duplicate
			found := false
			for _, existing := range services {
				if existing == svc {
					found = true
					break
				}
			}
			if !found {
				services = append(services, svc)
			}
		}
	} else {
		for _, arg := range args {
			if svc, ok := serviceMap[arg]; ok {
				services = append(services, svc)
			} else {
				fmt.Fprintf(os.Stderr, "Unknown service '%s', skipping\n", arg)
			}
		}
	}

	if len(services) == 0 {
		fmt.Fprint(os.Stderr, format.Error("No valid services specified\n"))
		return
	}

	// Create v2 bundle directory structure: {outputDir}/mdb/redis/
	mdbDir := filepath.Join(outputDir, "mdb")
	redisDir := filepath.Join(mdbDir, "redis")
	if err := os.MkdirAll(redisDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to create output directory: %v\n"), err)
		return
	}

	// Compute boot timestamp from /proc/uptime for monotonic→wallclock conversion,
	// plus the rest of the per-host metadata fields.
	now := time.Now().UTC()
	var (
		bootTimestamp string
		uptimeSeconds float64
	)
	if uptimeBytes, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(uptimeBytes))
		if len(fields) >= 1 {
			if secs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				uptimeSeconds = secs
				bootTime := now.Add(-time.Duration(secs * float64(time.Second))).UTC()
				bootTimestamp = bootTime.Format(time.RFC3339)
			}
		}
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		if hn, err := os.ReadFile("/etc/hostname"); err == nil {
			hostname = strings.TrimSpace(string(hn))
		}
	}

	var kernelRelease string
	if kr, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernelRelease = strings.TrimSpace(string(kr))
	}

	osReleaseID, osReleaseVersion := readOSRelease("/etc/os-release")

	if !*JSONOutput {
		fmt.Printf("%s Extracting logs to %s\n", format.Info("→"), tarballPath)
	}

	// Extract service logs into mdb/
	var journalBytes int64
	for _, svc := range services {
		n, err := extractServiceLogs(svc, mdbDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Failed to extract %s: %v\n"), svc, err)
			continue
		}
		journalBytes += n
		if !*JSONOutput {
			fmt.Printf("  %s %s\n", format.Success("✓"), svc)
		}
	}

	// Collect dmesg into mdb/
	if !*JSONOutput {
		fmt.Printf("%s Collecting dmesg\n", format.Info("→"))
	}
	var dmesgBytes int64
	if n, err := captureDmesg(mdbDir); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Failed to collect dmesg: %v\n"), err)
	} else {
		dmesgBytes = n
		if !*JSONOutput {
			fmt.Printf("  %s dmesg.log\n", format.Success("✓"))
		}
	}

	// Capture Redis snapshots into mdb/redis/
	if !*JSONOutput {
		fmt.Printf("%s Capturing Redis snapshots\n", format.Info("→"))
	}
	capturedCount := captureRedisSnapshots(redisDir)
	if !*JSONOutput {
		fmt.Printf("  %s %d keys captured\n", format.Success("✓"), capturedCount)
	}

	// Per-host metadata (mdb/metadata.json)
	hostMetadata := map[string]interface{}{
		"host":               "mdb",
		"hostname":           hostname,
		"boot_timestamp":     bootTimestamp,
		"uptime_seconds":     uptimeSeconds,
		"kernel_release":     kernelRelease,
		"os_release_id":      osReleaseID,
		"os_release_version": osReleaseVersion,
		"journal_bytes":      journalBytes,
		"dmesg_bytes":        dmesgBytes,
		"collector":          "local",
	}
	if data, err := json.MarshalIndent(hostMetadata, "", "  "); err == nil {
		if werr := os.WriteFile(filepath.Join(mdbDir, "metadata.json"), data, 0644); werr != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Failed to write host metadata: %v\n"), werr)
		}
	}

	// Top-level bundle metadata (format v2)
	metadata := map[string]interface{}{
		"version":      2,
		"collected_at": now.Format(time.RFC3339),
		"since":        logsSince,
		"until":        logsUntil,
		"request_id":   "",
		"hosts":        []string{"mdb"},
		"tool": map[string]string{
			"name":    "lsc",
			"version": ToolVersion,
		},
		"services": services,
		"priority": logsPriority,
	}

	metadataPath := filepath.Join(outputDir, "metadata.json")
	if data, err := json.MarshalIndent(metadata, "", "  "); err == nil {
		os.WriteFile(metadataPath, data, 0644)
	}

	// Create tarball
	if !*JSONOutput {
		fmt.Printf("%s Creating compressed archive\n", format.Info("→"))
	}
	if err := createTarball(outputDir, bundleName, tarballPath); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Failed to create tarball: %v\n"), err)
	} else if !*JSONOutput {
		fmt.Printf("  %s %s\n", format.Success("✓"), filepath.Base(tarballPath))
	}

	if *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command":         "logs-extract",
			"status":          "success",
			"bundle_version":  2,
			"tarball":         tarballPath,
			"services_count":  len(services),
			"redis_snapshots": capturedCount,
			"journal_bytes":   journalBytes,
			"dmesg_bytes":     dmesgBytes,
		})
		fmt.Println(string(output))
	} else {
		fmt.Printf("\n%s Logs extracted successfully\n", format.Success("✓"))
		fmt.Printf("  Archive: %s\n", tarballPath)
	}
}

// extractServiceLogs writes journalctl output for `service` to {hostDir}/{service}.log
// and returns the number of bytes written.
func extractServiceLogs(service, hostDir string) (int64, error) {
	args := []string{"-u", service, "--no-pager", "-o", "short-monotonic"}

	if logsSince != "" {
		args = append(args, "--since", convertDurationToJournalctl(logsSince))
	}
	if logsUntil != "" {
		args = append(args, "--until", convertDurationToJournalctl(logsUntil))
	}
	if logsPriority != "" {
		args = append(args, "--priority", logsPriority)
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	logFile := filepath.Join(hostDir, service+".log")
	if err := os.WriteFile(logFile, output, 0644); err != nil {
		return 0, err
	}
	return int64(len(output)), nil
}

// captureDmesg writes dmesg output to {hostDir}/dmesg.log and returns bytes written.
func captureDmesg(hostDir string) (int64, error) {
	var args []string

	if logsSince != "" {
		args = append(args, "--since", convertDurationToJournalctl(logsSince))
	}

	cmd := exec.Command("dmesg", args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	logFile := filepath.Join(hostDir, "dmesg.log")
	if err := os.WriteFile(logFile, output, 0644); err != nil {
		return 0, err
	}
	return int64(len(output)), nil
}

// convertDurationToJournalctl converts duration strings like "1h", "24h", "1d"
// to journalctl-compatible format like "1 hour ago", "24 hours ago", "1 day ago"
func convertDurationToJournalctl(duration string) string {
	// If it already looks like an absolute timestamp or journalctl format, return as-is
	if strings.Contains(duration, " ") || strings.Contains(duration, "-") || strings.Contains(duration, ":") {
		return duration
	}

	// Parse common duration formats
	duration = strings.TrimSpace(duration)
	if duration == "" {
		return duration
	}

	// Handle hours (1h, 24h)
	if strings.HasSuffix(duration, "h") {
		hours := strings.TrimSuffix(duration, "h")
		if hours == "1" {
			return "1 hour ago"
		}
		return hours + " hours ago"
	}

	// Handle days (1d, 7d)
	if strings.HasSuffix(duration, "d") {
		days := strings.TrimSuffix(duration, "d")
		if days == "1" {
			return "1 day ago"
		}
		return days + " days ago"
	}

	// Handle weeks (1w, 2w)
	if strings.HasSuffix(duration, "w") {
		weeks := strings.TrimSuffix(duration, "w")
		if weeks == "1" {
			return "1 week ago"
		}
		return weeks + " weeks ago"
	}

	// Handle minutes (1m, 30m)
	if strings.HasSuffix(duration, "m") {
		minutes := strings.TrimSuffix(duration, "m")
		if minutes == "1" {
			return "1 minute ago"
		}
		return minutes + " minutes ago"
	}

	// If no known suffix, return as-is (might be absolute timestamp)
	return duration
}

// captureRedisSnapshots writes HGETALL dumps for `redisKeys` into the given
// redis directory. Filename is the key with ':' replaced by '-', no prefix.
func captureRedisSnapshots(redisDir string) int {
	count := 0

	for _, key := range redisKeys {
		data, err := RedisClient.HGetAll(key)
		if err != nil || len(data) == 0 {
			continue
		}

		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			continue
		}

		filename := strings.ReplaceAll(key, ":", "-") + ".json"
		snapshotFile := filepath.Join(redisDir, filename)

		if err := os.WriteFile(snapshotFile, jsonData, 0644); err == nil {
			count++
		}
	}

	return count
}

// readOSRelease parses /etc/os-release and returns (ID, VERSION_ID), stripping
// surrounding single/double quotes. Missing fields return empty strings.
func readOSRelease(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}

	var id, versionID string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := line[:eq]
		val := strings.Trim(line[eq+1:], `"'`)
		switch key {
		case "ID":
			id = val
		case "VERSION_ID":
			versionID = val
		}
	}
	return id, versionID
}

func createTarball(sourceDir, bundleName, tarballPath string) error {
	file, err := os.Create(tarballPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory itself
		if path == sourceDir {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Make path relative to source directory
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join(bundleName, relPath)

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if it's a regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})
}

func init() {
	LogsCmd.Flags().StringVar(&logsSince, "since", "24h", "Start time for logs (journalctl format)")
	LogsCmd.Flags().StringVar(&logsUntil, "until", "", "End time for logs (default: now)")
	LogsCmd.Flags().StringVar(&logsOutput, "output", "", "Directory to write the .tar.gz into (default: /data/log-bundles)")
	LogsCmd.Flags().StringVar(&logsPriority, "priority", "", "Log level filter (err, warning, info, debug)")
}
