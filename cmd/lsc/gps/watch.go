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
	Long:  `Subscribe to GPS updates and display changes in real-time.`,
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

		// Subscribe to GPS updates
		pubsub := RedisClient.Subscribe(ctx, "gps")
		defer pubsub.Close()

		if JSONOutput == nil || !*JSONOutput {
			fmt.Println(format.Success("Watching GPS updates... (Ctrl+C to stop)"))
			fmt.Println()
		}

		ch := pubsub.Channel()

		// Print initial status
		printGPSUpdate(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				// GPS hash was updated, fetch and display
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
	// One-line format: timestamp | lat,lon | speed | course | accuracy
	timestamp := "N/A"
	if ts, ok := gpsData["updated"]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = t.Format("15:04:05")
		}
	}

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
			course = fmt.Sprintf("%.0f° %s", courseVal, degreesToCardinal(courseVal))
		}
	}

	accuracy := "N/A"
	if eph, ok := gpsData["eph"]; ok {
		if ephVal, err := strconv.ParseFloat(eph, 64); err == nil {
			accuracy = formatAccuracy(ephVal)
		}
	}

	fmt.Printf("%s | %s,%s | %s km/h | %s | %s\n",
		format.Dim(timestamp),
		lat, lon,
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

	accuracy := "N/A"
	if eph, ok := gpsData["eph"]; ok {
		if ephVal, err := strconv.ParseFloat(eph, 64); err == nil {
			accuracy = formatAccuracy(ephVal)
		}
	}

	fmt.Printf("[%s] %s %s | %s,%s | %s km/h | %s | Acc: %s\n",
		format.Dim(timestamp),
		format.ColorizeState(state),
		formatFixType(fixType),
		lat, lon,
		speed,
		course,
		accuracy,
	)
}

func init() {
	watchCmd.Flags().BoolVar(&watchCompact, "compact", false, "Use compact one-line format")
	GpsCmd.AddCommand(watchCmd)
}
