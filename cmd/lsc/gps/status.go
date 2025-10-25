package gps

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GPS status",
	Long:  `Display current GPS fix status, position, and accuracy information.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch GPS data
		gpsData, err := RedisClient.HGetAll("gps")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch GPS data: %v\n"), err)
			}
			return
		}

		if len(gpsData) == 0 {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": "No GPS data available",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Warning("No GPS data available"))
			}
			return
		}

		// If JSON output is requested
		if JSONOutput != nil && *JSONOutput {
			parseFloat := func(s string) float64 {
				v, _ := strconv.ParseFloat(s, 64)
				return v
			}

			output := map[string]interface{}{
				"connected": gpsData["connected"] == "1",
				"active":    gpsData["active"] == "1",
				"state":     gpsData["state"],
				"fix_type":  gpsData["fix"],
			}

			// Add position if available
			if gpsData["state"] == "fix-established" || gpsData["state"] == "tracking" {
				output["position"] = map[string]interface{}{
					"latitude":  parseFloat(gpsData["latitude"]),
					"longitude": parseFloat(gpsData["longitude"]),
					"altitude":  parseFloat(gpsData["altitude"]),
					"speed":     parseFloat(gpsData["speed"]),
					"course":    parseFloat(gpsData["course"]),
				}
				output["accuracy"] = map[string]interface{}{
					"eph":     parseFloat(gpsData["eph"]),
					"quality": parseFloat(gpsData["quality"]),
					"hdop":    parseFloat(gpsData["hdop"]),
					"pdop":    parseFloat(gpsData["pdop"]),
					"vdop":    parseFloat(gpsData["vdop"]),
				}
				output["timestamp"] = gpsData["timestamp"]
				output["updated"] = gpsData["updated"]
			}

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		// Display GPS status
		format.PrintSection("GPS Status")

		// Connection and fix status
		connected := gpsData["connected"] == "1"
		active := gpsData["active"] == "1"
		state := gpsData["state"]
		fixType := gpsData["fix"]

		if connected {
			format.PrintKV("Connected", format.Success("Yes"))
		} else {
			format.PrintKV("Connected", format.Error("No"))
		}

		if active {
			format.PrintKV("Active", format.Success("Yes"))
		} else {
			format.PrintKV("Active", format.Warning("No"))
		}

		format.PrintKV("State", format.ColorizeState(state))
		format.PrintKV("Fix Type", formatFixType(fixType))

		// Position information
		if state == "fix-established" || state == "tracking" {
			format.PrintSubsection("Position")
			format.PrintKV("Latitude", fmt.Sprintf("%s°", gpsData["latitude"]))
			format.PrintKV("Longitude", fmt.Sprintf("%s°", gpsData["longitude"]))
			format.PrintKV("Altitude", fmt.Sprintf("%s m", gpsData["altitude"]))

			if speed, ok := gpsData["speed"]; ok {
				speedVal, _ := strconv.ParseFloat(speed, 64)
				format.PrintKV("Speed", fmt.Sprintf("%.1f km/h", speedVal))
			}

			if course, ok := gpsData["course"]; ok {
				courseVal, _ := strconv.ParseFloat(course, 64)
				format.PrintKV("Course", fmt.Sprintf("%.1f° (%s)", courseVal, degreesToCardinal(courseVal)))
			}

			// Accuracy information
			format.PrintSubsection("Accuracy")

			if eph, ok := gpsData["eph"]; ok {
				ephVal, _ := strconv.ParseFloat(eph, 64)
				format.PrintKV("Horizontal Error", formatAccuracy(ephVal))
			}

			if quality, ok := gpsData["quality"]; ok {
				qualityVal, _ := strconv.ParseFloat(quality, 64)
				format.PrintKV("Quality", formatQuality(qualityVal))
			}

			if _, ok := gpsData["hdop"]; ok {
				format.PrintKV("HDOP", gpsData["hdop"])
			}
			if _, ok := gpsData["pdop"]; ok {
				format.PrintKV("PDOP", gpsData["pdop"])
			}
			if _, ok := gpsData["vdop"]; ok {
				format.PrintKV("VDOP", gpsData["vdop"])
			}

			// Timestamp
			format.PrintSubsection("Time")
			if timestamp, ok := gpsData["timestamp"]; ok {
				if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
					format.PrintKV("GPS Time", t.Format("2006-01-02 15:04:05 MST"))
				} else {
					format.PrintKV("GPS Time", timestamp)
				}
			}
			if updated, ok := gpsData["updated"]; ok {
				if t, err := time.Parse(time.RFC3339, updated); err == nil {
					format.PrintKV("Last Update", t.Format("2006-01-02 15:04:05 MST"))
				} else {
					format.PrintKV("Last Update", updated)
				}
			}
		}

		fmt.Println()
	},
}

func formatFixType(fixType string) string {
	switch fixType {
	case "3d":
		return format.Success("3D Fix")
	case "2d":
		return format.Warning("2D Fix")
	case "none", "":
		return format.Error("No Fix")
	default:
		return fixType
	}
}

func formatAccuracy(meters float64) string {
	s := fmt.Sprintf("%.1f m", meters)
	if meters < 10 {
		return format.Success(s)
	} else if meters < 50 {
		return format.Warning(s)
	} else {
		return format.Error(s)
	}
}

func formatQuality(quality float64) string {
	// Lower quality values are better
	s := fmt.Sprintf("%.3f", quality)
	if quality < 0.01 {
		return format.Success(s)
	} else if quality < 0.1 {
		return format.Warning(s)
	} else {
		return format.Error(s)
	}
}

func degreesToCardinal(degrees float64) string {
	// Normalize to 0-360
	for degrees < 0 {
		degrees += 360
	}
	for degrees >= 360 {
		degrees -= 360
	}

	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((degrees + 11.25) / 22.5)
	if index >= len(directions) {
		index = 0
	}
	return directions[index]
}

func init() {
	GpsCmd.AddCommand(statusCmd)
}
