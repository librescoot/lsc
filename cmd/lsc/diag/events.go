package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	eventsSince  string
	eventsCount  int
	eventsFollow bool
	eventsFilter string
	eventsJSON   bool
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "View fault event stream",
	Long:  `Display fault events from the events:faults stream with optional filtering and follow mode.`,
	Run: func(cmd *cobra.Command, args []string) {
		var filterRegex *regexp.Regexp
		if eventsFilter != "" {
			var err error
			filterRegex, err = regexp.Compile(eventsFilter)
			if err != nil {
				fmt.Fprintf(os.Stderr, format.Error("Invalid filter regex: %v\n"), err)
				return
			}
		}

		ctx := context.Background()
		if eventsFollow {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle Ctrl+C
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				cancel()
			}()

			followEvents(ctx, filterRegex)
		} else {
			showEvents(ctx, filterRegex)
		}
	},
}

func showEvents(ctx context.Context, filterRegex *regexp.Regexp) {
	// Determine the start ID based on --since
	startID := "0"
	if eventsSince != "" {
		duration, err := parseDuration(eventsSince)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Invalid duration '%s': %v\n"), eventsSince, err)
			return
		}
		// Calculate the approximate stream ID from timestamp
		sinceTime := time.Now().Add(-duration)
		startID = fmt.Sprintf("%d-0", sinceTime.UnixMilli())
	}

	// Read from stream
	streams, err := RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{"events:faults", startID},
		Count:   int64(eventsCount),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to read events: %v\n"), err)
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		fmt.Println(format.Dim("No events found"))
		return
	}

	// Process and display events
	events := streams[0].Messages
	for _, msg := range events {
		if !matchesFilter(msg, filterRegex) {
			continue
		}
		printEvent(msg)
	}
}

func followEvents(ctx context.Context, filterRegex *regexp.Regexp) {
	// Start from the latest event
	lastID := "$"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read with blocking
		streams, err := RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{"events:faults", lastID},
			Count:   10,
			Block:   1 * time.Second,
		})

		if err != nil {
			// Timeout is normal in block mode
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "nil") {
				continue
			}
			fmt.Fprintf(os.Stderr, format.Error("Error reading events: %v\n"), err)
			return
		}

		if len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		// Process new events
		events := streams[0].Messages
		for _, msg := range events {
			if matchesFilter(msg, filterRegex) {
				printEvent(msg)
			}
			lastID = msg.ID
		}
	}
}

func printEvent(msg redis.XMessage) {
	if eventsJSON {
		printEventJSON(msg)
		return
	}

	// Parse timestamp from ID (format: "milliseconds-sequence")
	idParts := strings.Split(msg.ID, "-")
	ts := "N/A"
	if len(idParts) > 0 {
		if ms, err := strconv.ParseInt(idParts[0], 10, 64); err == nil {
			t := time.UnixMilli(ms)
			ts = t.Format("2006-01-02 15:04:05")
		}
	}

	group := getField(msg.Values, "group")
	code := getField(msg.Values, "code")
	description := getField(msg.Values, "description")

	// Determine severity/color based on group or code
	severity := determineSeverity(group, code)

	fmt.Printf("%s %s [%s:%s] %s\n",
		format.Dim(ts),
		severity,
		format.Warning(group),
		format.Warning(code),
		description,
	)
}

func printEventJSON(msg redis.XMessage) {
	// Parse timestamp from ID
	idParts := strings.Split(msg.ID, "-")
	var timestamp int64
	if len(idParts) > 0 {
		timestamp, _ = strconv.ParseInt(idParts[0], 10, 64)
	}

	event := map[string]interface{}{
		"id":          msg.ID,
		"timestamp":   timestamp,
		"group":       getField(msg.Values, "group"),
		"code":        getField(msg.Values, "code"),
		"description": getField(msg.Values, "description"),
	}

	// Add any additional fields
	for k, v := range msg.Values {
		if k != "group" && k != "code" && k != "description" {
			event[k] = v
		}
	}

	jsonBytes, _ := json.Marshal(event)
	fmt.Println(string(jsonBytes))
}

func matchesFilter(msg redis.XMessage, filterRegex *regexp.Regexp) bool {
	if filterRegex == nil {
		return true
	}

	group := getField(msg.Values, "group")
	code := getField(msg.Values, "code")
	description := getField(msg.Values, "description")
	combined := fmt.Sprintf("%s %s %s", group, code, description)

	return filterRegex.MatchString(combined)
}

func getField(values map[string]interface{}, key string) string {
	if val, ok := values[key]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func determineSeverity(group, code string) string {
	// Use red for critical errors, yellow for warnings
	if strings.Contains(strings.ToLower(group), "battery") ||
		strings.Contains(strings.ToLower(code), "critical") ||
		strings.Contains(strings.ToLower(code), "error") {
		return format.Error("ERROR")
	}
	return format.Warning("WARN")
}

func parseDuration(s string) (time.Duration, error) {
	// Support formats like: 1h, 24h, 7d, 1w
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Handle days and weeks
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		weeks, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	// Let time.ParseDuration handle standard formats (h, m, s)
	return time.ParseDuration(s)
}

func init() {
	eventsCmd.Flags().StringVar(&eventsSince, "since", "", "Show events since duration ago (1h, 24h, 7d, 1w)")
	eventsCmd.Flags().IntVar(&eventsCount, "count", 50, "Maximum number of events to show")
	eventsCmd.Flags().BoolVar(&eventsFollow, "follow", false, "Follow the stream (like tail -f)")
	eventsCmd.Flags().StringVar(&eventsFilter, "filter", "", "Filter events by regex pattern")
	eventsCmd.Flags().BoolVar(&eventsJSON, "json", false, "Output events in JSON format")

	DiagCmd.AddCommand(eventsCmd)
}
