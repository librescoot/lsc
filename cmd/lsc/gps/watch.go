package gps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var (
	watchCompact bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch GPS updates in real-time",
	Long:  `Poll GPS updates and display changes in real-time.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle Ctrl+C
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			if JSONOutput == nil || !*JSONOutput {
				fmt.Println(format.Dim("\nStopping GPS watch..."))
			}
			cancel()
		}()

		if JSONOutput == nil || !*JSONOutput {
			fmt.Println(format.Success("Watching GPS updates... (Ctrl+C to stop)"))
			fmt.Println()
		}

		// Print initial status
		printGPSUpdate(ctx)

		// Poll for updates every second
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Poll GPS hash and display
				printGPSUpdate(ctx)
			}
		}
	},
}

func printGPSUpdate(ctx context.Context) {
	gpsData, err := RedisClient.HGetAllWithContext(ctx, "gps")
	if err != nil {
		return
	}

	if JSONOutput != nil && *JSONOutput {
		printJSONUpdate(gpsData)
	} else if watchCompact {
		printCompactUpdate(gpsData)
	} else {
		printFullUpdate(gpsData)
	}
}

func printJSONUpdate(gpsData map[string]string) {
	parseFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	output := map[string]interface{}{
		"timestamp":  time.Now().Unix(),
		"connected":  gpsData["connected"] == "1",
		"active":     gpsData["active"] == "1",
		"state":      gpsData["state"],
		"fix_type":   gpsData["fix"],
		"latitude":   parseFloat(gpsData["latitude"]),
		"longitude":  parseFloat(gpsData["longitude"]),
		"altitude":   parseFloat(gpsData["altitude"]),
		"speed":      parseFloat(gpsData["speed"]),
		"course":     parseFloat(gpsData["course"]),
		"eph":        parseFloat(gpsData["eph"]),
		"quality":    parseFloat(gpsData["quality"]),
		"hdop":       parseFloat(gpsData["hdop"]),
		"pdop":       parseFloat(gpsData["pdop"]),
		"vdop":       parseFloat(gpsData["vdop"]),
		"gps_time":   gpsData["timestamp"],
		"updated":    gpsData["updated"],
	}

	jsonBytes, _ := json.Marshal(output)
	fmt.Println(string(jsonBytes))
}

func printCompactUpdate(gpsData map[string]string) {
	// One-line format: timestamp | lat,lon | alt | speed | course | accuracy
	timestamp := "N/A"
	if ts, ok := gpsData["updated"]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = t.Format("15:04:05")
		}
	}

	lat := gpsData["latitude"]
	lon := gpsData["longitude"]

	altitude := "N/A"
	if alt, ok := gpsData["altitude"]; ok {
		if altVal, err := strconv.ParseFloat(alt, 64); err == nil {
			altitude = fmt.Sprintf("%.0fm", altVal)
		}
	}

	speed := "0.0"
	if s, ok := gpsData["speed"]; ok {
		if speedVal, err := strconv.ParseFloat(s, 64); err == nil {
			speed = fmt.Sprintf("%.1f", speedVal)
		}
	}

	course := "---"
	if c, ok := gpsData["course"]; ok {
		if courseVal, err := strconv.ParseFloat(c, 64); err == nil {
			course = fmt.Sprintf("%.0f° %s", courseVal, degreesToCardinal(courseVal))
		}
	}

	accuracy := "N/A"
	if eph, ok := gpsData["eph"]; ok {
		if ephVal, err := strconv.ParseFloat(eph, 64); err == nil {
			accuracy = formatAccuracy(ephVal)
		}
	}

	fmt.Printf("%s | %s,%s | %s | %s km/h | %s | %s\n",
		format.Dim(timestamp),
		lat, lon,
		altitude,
		speed,
		course,
		accuracy,
	)
}

func printFullUpdate(gpsData map[string]string) {
	timestamp := time.Now().Format("15:04:05")

	state := gpsData["state"]
	fixType := gpsData["fix"]
	lat := gpsData["latitude"]
	lon := gpsData["longitude"]

	speed := "0.0"
	if s, ok := gpsData["speed"]; ok {
		if speedVal, err := strconv.ParseFloat(s, 64); err == nil {
			speed = fmt.Sprintf("%.1f", speedVal)
		}
	}

	course := "---"
	if c, ok := gpsData["course"]; ok {
		if courseVal, err := strconv.ParseFloat(c, 64); err == nil {
			course = fmt.Sprintf("%.1f° (%s)", courseVal, degreesToCardinal(courseVal))
		}
	}

	altitude := "N/A"
	if alt, ok := gpsData["altitude"]; ok {
		if altVal, err := strconv.ParseFloat(alt, 64); err == nil {
			altitude = fmt.Sprintf("%.1f m", altVal)
		}
	}

	accuracy := "N/A"
	if eph, ok := gpsData["eph"]; ok {
		if ephVal, err := strconv.ParseFloat(eph, 64); err == nil {
			accuracy = formatAccuracy(ephVal)
		}
	}

	quality := gpsData["quality"]
	hdop := gpsData["hdop"]
	pdop := gpsData["pdop"]
	vdop := gpsData["vdop"]

	gpsTime := "N/A"
	if ts, ok := gpsData["timestamp"]; ok && ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			gpsTime = t.Format("15:04:05")
		}
	}

	// Show state if no fix or in error state
	statePrefix := ""
	if fixType == "" || fixType == "none" || fixType == "unknown" || state == "error" || state == "no-fix" {
		statePrefix = format.ColorizeState(state) + " "
	}

	// Single line with all info
	fmt.Printf("[%s] %s%s | %s,%s | ▲ %s | %s km/h | %s | Acc: %s | Q: %s | DOP: %s/%s/%s | T: %s\n",
		format.Dim(timestamp),
		statePrefix,
		formatFixType(fixType),
		lat, lon,
		altitude,
		speed,
		course,
		accuracy,
		quality,
		hdop, pdop, vdop,
		format.Dim(gpsTime),
	)
}

func init() {
	watchCmd.Flags().BoolVar(&watchCompact, "compact", false, "Use compact one-line format")
	GpsCmd.AddCommand(watchCmd)
}
