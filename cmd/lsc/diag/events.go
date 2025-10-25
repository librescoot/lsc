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
	eventsSince   string
	eventsUntil   string
	eventsCount   int
	eventsFollow  bool
	eventsFilter  string
	eventsReverse bool
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "View fault event stream",
	Long: `Display fault events from the events:faults stream with filtering and follow mode.

Time range filtering (similar to journalctl):
  --since <duration>   Show events since duration ago (e.g., 1h, 24h, 7d, 1w)
  --until <duration>   Show events until duration ago (limits upper bound)

Output control (similar to tail):
  -n, --lines <N>      Show at most N events (default 50)
  -r, --reverse        Show newest events first (default: oldest first)
  -f, --follow         Follow the stream in real-time

Filtering:
  --filter <regex>     Filter events by regex pattern (matches group, code, or description)

Examples:
  lsc events --since 1h                 # Last hour of events
  lsc events --since 24h --until 1h     # Events between 24h and 1h ago
  lsc events -n 10 -r                   # Last 10 events, newest first
  lsc events -f                         # Follow events in real-time
  lsc events --filter "battery"         # Events containing "battery"`,
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
	var sinceTime time.Time
	if eventsSince != "" {
		duration, err := parseDuration(eventsSince)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Invalid duration '%s': %v\n"), eventsSince, err)
			return
		}
		// Calculate the approximate stream ID from timestamp
		sinceTime = time.Now().Add(-duration)
		startID = fmt.Sprintf("%d-0", sinceTime.UnixMilli())
	}

	// Determine the end time based on --until
	var untilTime time.Time
	if eventsUntil != "" {
		duration, err := parseDuration(eventsUntil)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Invalid duration '%s': %v\n"), eventsUntil, err)
			return
		}
		untilTime = time.Now().Add(-duration)
	}

	// Read from stream (get more than count to allow for filtering)
	readCount := int64(eventsCount * 2)
	if readCount < 100 {
		readCount = 100
	}
	streams, err := RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{"events:faults", startID},
		Count:   readCount,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to read events: %v\n"), err)
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		fmt.Println(format.Dim("No events found"))
		return
	}

	// Filter and collect events
	var filteredEvents []redis.XMessage
	for _, msg := range streams[0].Messages {
		// Parse timestamp from message ID
		idParts := strings.Split(msg.ID, "-")
		if len(idParts) > 0 {
			if ms, err := strconv.ParseInt(idParts[0], 10, 64); err == nil {
				eventTime := time.UnixMilli(ms)

				// Filter by time range
				if !sinceTime.IsZero() && eventTime.Before(sinceTime) {
					continue
				}
				if !untilTime.IsZero() && eventTime.After(untilTime) {
					continue
				}
			}
		}

		// Filter by regex
		if !matchesFilter(msg, filterRegex) {
			continue
		}

		filteredEvents = append(filteredEvents, msg)
	}

	// Apply reverse if requested
	if eventsReverse {
		// Reverse the slice
		for i, j := 0, len(filteredEvents)-1; i < j; i, j = i+1, j-1 {
			filteredEvents[i], filteredEvents[j] = filteredEvents[j], filteredEvents[i]
		}
	}

	// Limit to count
	if len(filteredEvents) > eventsCount {
		filteredEvents = filteredEvents[:eventsCount]
	}

	// Display events
	if len(filteredEvents) == 0 {
		fmt.Println(format.Dim("No events found matching criteria"))
		return
	}

	for _, msg := range filteredEvents {
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
	if JSONOutput != nil && *JSONOutput {
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
	eventsCmd.Flags().StringVar(&eventsUntil, "until", "", "Show events until duration ago (1h, 24h, 7d, 1w)")
	eventsCmd.Flags().IntVarP(&eventsCount, "lines", "n", 50, "Maximum number of events to show")
	eventsCmd.Flags().BoolVarP(&eventsFollow, "follow", "f", false, "Follow the stream (like tail -f)")
	eventsCmd.Flags().BoolVarP(&eventsReverse, "reverse", "r", false, "Show newest events first")
	eventsCmd.Flags().StringVar(&eventsFilter, "filter", "", "Filter events by regex pattern")

	// Keep --count as deprecated alias for --lines
	eventsCmd.Flags().IntVar(&eventsCount, "count", 50, "Maximum number of events to show (deprecated: use -n/--lines)")
	eventsCmd.Flags().MarkDeprecated("count", "use -n or --lines instead")

	DiagCmd.AddCommand(eventsCmd)
}
