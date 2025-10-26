package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool

	// Flags
	monitorDuration string
	monitorInterval string
	monitorOutput   string
	monitorFormat   string
)

// Subsystem names
var subsystems = []string{
	"gps", "battery", "vehicle", "motor", "power", "modem", "events", "all",
}

// Session metadata
type SessionMetadata struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    string    `json:"duration"`
	Interval    string    `json:"interval"`
	Subsystems  []string  `json:"subsystems"`
	RecordCount map[string]int `json:"record_count"`
}

var MonitorCmd = &cobra.Command{
	Use:       "monitor <subsystems...>",
	Short:     "Record real-time metrics over time",
	Long: `Record scooter metrics to timestamped files for analysis.

Available subsystems:
  gps      - GPS coordinates and speed
  battery  - Battery states (all connected batteries)
  vehicle  - Vehicle state changes
  motor    - Motor/ECU metrics
  power    - Power manager state
  modem    - Modem and internet connectivity
  events   - Fault event stream
  all      - All of the above

The command will:
  1. Record metrics at regular intervals
  2. Write timestamped JSONL or CSV files
  3. Generate metadata file
  4. Create compressed .tar.gz archive

Examples:
  lsc monitor gps --duration 1h
  lsc monitor battery vehicle --duration 10m --interval 5s
  lsc monitor all --duration 30m --output /data/debug-session
  lsc monitor gps battery --format csv --duration 5m`,
	Args:      cobra.MinimumNArgs(1),
	ValidArgs: subsystems,
	Run:       runMonitor,
}

// SetRedisClient sets the Redis client for monitor commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for monitor commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

func runMonitor(cmd *cobra.Command, args []string) {
	// Parse duration
	duration, err := parseDuration(monitorDuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Invalid duration '%s': %v\n"), monitorDuration, err)
		return
	}

	// Parse interval
	interval, err := parseDuration(monitorInterval)
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Invalid interval '%s': %v\n"), monitorInterval, err)
		return
	}

	// Determine output directory
	outputDir := monitorOutput
	if outputDir == "" {
		outputDir = fmt.Sprintf("/data/monitor-%s", time.Now().Format("2006-01-02-15-04"))
	}

	// Determine which subsystems to monitor
	selectedSubsystems := expandSubsystems(args)
	if len(selectedSubsystems) == 0 {
		fmt.Fprintf(os.Stderr, format.Error("No valid subsystems specified\n"))
		return
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to create output directory: %v\n"), err)
		return
	}

	if !*JSONOutput {
		fmt.Printf("%s Recording metrics to %s\n", format.Info("→"), outputDir)
		fmt.Printf("  Duration: %s\n", monitorDuration)
		fmt.Printf("  Interval: %s\n", monitorInterval)
		fmt.Printf("  Subsystems: %v\n", selectedSubsystems)
		fmt.Println()
	}

	// Setup context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if !*JSONOutput {
			fmt.Println("\nShutting down gracefully...")
		}
		cancel()
	}()

	// Track record counts
	recordCounts := make(map[string]*int)
	var mu sync.Mutex

	// Start recorders
	var wg sync.WaitGroup
	startTime := time.Now()

	for _, subsystem := range selectedSubsystems {
		count := 0
		recordCounts[subsystem] = &count

		wg.Add(1)
		switch subsystem {
		case "gps":
			go recordGPS(ctx, &wg, outputDir, interval, &count, &mu)
		case "battery":
			go recordBattery(ctx, &wg, outputDir, interval, &count, &mu)
		case "vehicle":
			go recordVehicle(ctx, &wg, outputDir, interval, &count, &mu)
		case "motor":
			go recordMotor(ctx, &wg, outputDir, interval, &count, &mu)
		case "power":
			go recordPower(ctx, &wg, outputDir, interval, &count, &mu)
		case "modem":
			go recordModem(ctx, &wg, outputDir, interval, &count, &mu)
		case "events":
			go recordEvents(ctx, &wg, outputDir, &count, &mu)
		}
	}

	// Progress updates
	if !*JSONOutput {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					mu.Lock()
					elapsed := time.Since(startTime)
					total := 0
					for _, count := range recordCounts {
						total += *count
					}
					mu.Unlock()
					fmt.Printf("\r%s Elapsed: %s | Records: %d", format.Info("→"), formatDuration(elapsed), total)
				}
			}
		}()
	}

	// Wait for all recorders to finish
	wg.Wait()
	endTime := time.Now()

	if !*JSONOutput {
		fmt.Println() // Clear progress line
	}

	// Write metadata
	metadata := SessionMetadata{
		StartTime:   startTime,
		EndTime:     endTime,
		Duration:    monitorDuration,
		Interval:    monitorInterval,
		Subsystems:  selectedSubsystems,
		RecordCount: make(map[string]int),
	}

	for subsystem, count := range recordCounts {
		metadata.RecordCount[subsystem] = *count
	}

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
	}

	// Print summary
	if *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command":        "monitor",
			"status":         "success",
			"output_dir":     outputDir,
			"tarball":        tarballPath,
			"duration":       endTime.Sub(startTime).Seconds(),
			"record_counts":  metadata.RecordCount,
			"total_records":  sumRecords(metadata.RecordCount),
		})
		fmt.Println(string(output))
	} else {
		fmt.Printf("\n%s Monitoring complete\n", format.Success("✓"))
		fmt.Printf("  Directory: %s\n", outputDir)
		fmt.Printf("  Archive:   %s\n", tarballPath)
		fmt.Printf("  Duration:  %s\n", formatDuration(endTime.Sub(startTime)))
		fmt.Printf("  Records:   %d\n", sumRecords(metadata.RecordCount))
	}
}

func expandSubsystems(args []string) []string {
	var result []string

	for _, arg := range args {
		if arg == "all" {
			return []string{"gps", "battery", "vehicle", "motor", "power", "modem", "events"}
		}
		// Validate subsystem
		valid := false
		for _, s := range subsystems {
			if s == arg {
				valid = true
				break
			}
		}
		if valid {
			result = append(result, arg)
		} else {
			fmt.Fprintf(os.Stderr, format.Warning("Unknown subsystem '%s', skipping\n"), arg)
		}
	}

	return result
}

func sumRecords(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func parseDuration(s string) (time.Duration, error) {
	// Support formats like: 1m, 5m, 1h, 24h, 100ms
	return time.ParseDuration(s)
}

func init() {
	MonitorCmd.Flags().StringVar(&monitorDuration, "duration", "1h", "Recording duration (1m, 5m, 1h, 24h)")
	MonitorCmd.Flags().StringVar(&monitorInterval, "interval", "1s", "Polling interval (100ms, 1s, 5s)")
	MonitorCmd.Flags().StringVar(&monitorOutput, "output", "", "Output directory (default: auto-generate)")
	MonitorCmd.Flags().StringVar(&monitorFormat, "format", "jsonl", "Output format (jsonl, csv)")
}
