package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var (
	watchFormat string
	watchFilter string
)

var watchCmd = &cobra.Command{
	Use:   "watch [channel...]",
	Short: "Monitor Redis pub/sub channels",
	Long: `Watch one or more Redis pub/sub channels in real-time.

Examples:
  # Watch vehicle state changes
  lsc watch vehicle

  # Watch multiple channels
  lsc watch vehicle alarm battery:0

  # Watch sensors with JSON output
  lsc watch bmx:sensors --format=json

  # Watch and filter messages
  lsc watch vehicle --filter="state|lock"

Useful channels:
  vehicle          - Vehicle state changes
  alarm            - Alarm status changes
  battery:0/1      - Battery state changes
  bmx:sensors      - Sensor data (10Hz when enabled)
  bmx:magnetometer - Magnetometer readings (5Hz)
  bmx:interrupt    - Motion detection events
  engine-ecu throttle  - Throttle events
  engine-ecu odometer  - Odometer updates
  gps              - GPS updates
  buttons          - Button press events
  dashboard        - Dashboard status
  settings         - Settings changes`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		channels := args

		// Compile filter regex if provided
		var filterRegex *regexp.Regexp
		if watchFilter != "" {
			var err error
			filterRegex, err = regexp.Compile(watchFilter)
			if err != nil {
				fmt.Fprintf(os.Stderr, format.Error("Invalid filter regex: %v\n"), err)
				return
			}
		}

		// Create context that can be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle Ctrl+C gracefully
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println(format.Dim("\nStopping..."))
			cancel()
		}()

		// Subscribe to channels
		pubsub := redisClient.Subscribe(ctx, channels...)
		defer pubsub.Close()

		// Print header
		if watchFormat != "json" && watchFormat != "raw" {
			fmt.Println(format.Info(fmt.Sprintf("Watching channels: %v", channels)))
			fmt.Println(format.Dim("Press Ctrl+C to stop\n"))
		}

		// Receive messages
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				// Apply filter if provided
				if filterRegex != nil {
					fullMessage := fmt.Sprintf("%s %s", msg.Channel, msg.Payload)
					if !filterRegex.MatchString(fullMessage) {
						continue
					}
				}

				// Format output based on mode
				switch watchFormat {
				case "json":
					printJSON(msg.Channel, msg.Payload)
				case "raw":
					fmt.Println(msg.Payload)
				default:
					printPretty(msg.Channel, msg.Payload)
				}
			}
		}
	},
}

func printPretty(channel, payload string) {
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Printf("[%s] [%s] %s\n",
		format.Dim(timestamp),
		format.Info(channel),
		payload)
}

func printJSON(channel, payload string) {
	output := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"channel":   channel,
		"payload":   payload,
	}

	// Try to parse payload as JSON
	var payloadJSON interface{}
	if err := json.Unmarshal([]byte(payload), &payloadJSON); err == nil {
		output["payload"] = payloadJSON
	}

	jsonBytes, _ := json.Marshal(output)
	fmt.Println(string(jsonBytes))
}

func init() {
	watchCmd.Flags().StringVar(&watchFormat, "format", "pretty", "Output format: pretty, json, raw")
	watchCmd.Flags().StringVar(&watchFilter, "filter", "", "Filter messages by regex")
	rootCmd.AddCommand(watchCmd)
}
