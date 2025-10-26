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
	"strings"
	"time"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool

	// Flags
	logsSince    string
	logsUntil    string
	logsOutput   string
	logsPriority string
)

// Service name mappings
var serviceMap = map[string]string{
	"vehicle":   "librescoot-vehicle.service",
	"battery":   "librescoot-battery.service",
	"ecu":       "librescoot-ecu.service",
	"motor":     "librescoot-ecu.service", // alias
	"modem":     "librescoot-modem.service",
	"pm":        "librescoot-pm.service",
	"power":     "librescoot-pm.service", // alias
	"update":    "librescoot-update.service",
	"settings":  "librescoot-settings.service",
	"keycard":   "librescoot-keycard.service",
	"bluetooth": "librescoot-bluetooth.service",
	"ble":       "librescoot-bluetooth.service", // alias
	"ums":       "librescoot-ums.service",
	"radio-gaga": "radio-gaga.service",
	"uplink":    "radio-gaga.service", // alias
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
	Use:   "logs <services...>",
	Short: "Extract service logs and system state",
	Long: `Extract systemd service logs and Redis snapshots for debugging and analysis.

Available services:
  vehicle, battery, ecu/motor, modem, pm/power, update, settings,
  keycard, bluetooth/ble, ums, radio-gaga/uplink, all

The command will:
  1. Extract journalctl logs for specified services
  2. Capture current Redis state snapshots
  3. Generate metadata file
  4. Create both unpacked directory and compressed .tar.gz archive

Examples:
  lsc logs vehicle --since 24h
  lsc logs all --since 1h --output /data/debug-session
  lsc logs battery ecu --since "2025-10-25 10:00" --until "2025-10-25 12:00"
  lsc logs all --since 1d --priority err`,
	Args: cobra.MinimumNArgs(1),
	Run:  runLogsExtract,
}

// SetRedisClient sets the Redis client for logs commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for logs commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

func runLogsExtract(cmd *cobra.Command, args []string) {
	// Determine output directory
	outputDir := logsOutput
	if outputDir == "" {
		outputDir = fmt.Sprintf("/data/logs-%s", time.Now().Format("2006-01-02-15-04"))
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
				fmt.Fprintf(os.Stderr, format.Warning("Unknown service '%s', skipping\n"), arg)
			}
		}
	}

	if len(services) == 0 {
		fmt.Fprintf(os.Stderr, format.Error("No valid services specified\n"))
		return
	}

	// Create output directory structure
	if err := os.MkdirAll(filepath.Join(outputDir, "logs"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to create output directory: %v\n"), err)
		return
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "snapshots"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to create snapshots directory: %v\n"), err)
		return
	}

	metadata := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"services":  services,
		"since":     logsSince,
		"until":     logsUntil,
		"priority":  logsPriority,
	}

	if !*JSONOutput {
		fmt.Printf("%s Extracting logs to %s\n", format.Info("→"), outputDir)
	}

	// Extract service logs
	for _, svc := range services {
		if err := extractServiceLogs(svc, outputDir); err != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Failed to extract %s: %v\n"), svc, err)
		} else if !*JSONOutput {
			fmt.Printf("  %s %s\n", format.Success("✓"), svc)
		}
	}

	// Capture Redis snapshots
	if !*JSONOutput {
		fmt.Printf("%s Capturing Redis snapshots\n", format.Info("→"))
	}
	capturedCount := captureRedisSnapshots(outputDir)
	if !*JSONOutput {
		fmt.Printf("  %s %d keys captured\n", format.Success("✓"), capturedCount)
	}
	metadata["redis_snapshots"] = capturedCount

	// Write metadata
	metadataPath := filepath.Join(outputDir, "metadata.json")
	if data, err := json.MarshalIndent(metadata, "", "  "); err == nil {
		os.WriteFile(metadataPath, data, 0644)
	}

	// Create tarball
	if !*JSONOutput {
		fmt.Printf("%s Creating compressed archive\n", format.Info("→"))
	}
	tarballPath := outputDir + ".tar.gz"
	if err := createTarball(outputDir, tarballPath); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Failed to create tarball: %v\n"), err)
	} else if !*JSONOutput {
		fmt.Printf("  %s %s\n", format.Success("✓"), filepath.Base(tarballPath))
	}

	if *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command":        "logs-extract",
			"status":         "success",
			"output_dir":     outputDir,
			"tarball":        tarballPath,
			"services_count": len(services),
			"redis_snapshots": capturedCount,
		})
		fmt.Println(string(output))
	} else {
		fmt.Printf("\n%s Logs extracted successfully\n", format.Success("✓"))
		fmt.Printf("  Directory: %s\n", outputDir)
		fmt.Printf("  Archive:   %s\n", tarballPath)
	}
}

func extractServiceLogs(service, outputDir string) error {
	args := []string{"-u", service, "--no-pager"}

	if logsSince != "" {
		args = append(args, "--since", logsSince)
	}
	if logsUntil != "" {
		args = append(args, "--until", logsUntil)
	}
	if logsPriority != "" {
		args = append(args, "--priority", logsPriority)
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Save to file
	logFile := filepath.Join(outputDir, "logs", service+".log")
	return os.WriteFile(logFile, output, 0644)
}

func captureRedisSnapshots(outputDir string) int {
	count := 0

	for _, key := range redisKeys {
		data, err := RedisClient.HGetAll(key)
		if err != nil || len(data) == 0 {
			continue
		}

		// Save as JSON
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			continue
		}

		// Sanitize key name for filename
		filename := strings.ReplaceAll(key, ":", "-") + ".json"
		snapshotFile := filepath.Join(outputDir, "snapshots", "redis-"+filename)

		if err := os.WriteFile(snapshotFile, jsonData, 0644); err == nil {
			count++
		}
	}

	return count
}

func createTarball(sourceDir, tarballPath string) error {
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

		// Skip the source directory itself and the tarball
		if path == sourceDir || strings.HasSuffix(path, ".tar.gz") {
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
		header.Name = filepath.Join(filepath.Base(sourceDir), relPath)

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
	LogsCmd.Flags().StringVar(&logsOutput, "output", "", "Output directory (default: auto-generate)")
	LogsCmd.Flags().StringVar(&logsPriority, "priority", "", "Log level filter (err, warning, info, debug)")
}
