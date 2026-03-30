package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

func colorizeOTAStatus(status string) string {
	switch status {
	case "idle":
		return format.Dim(status)
	case "downloading", "preparing":
		return format.Warning(status)
	case "installing":
		return format.Warning(status)
	case "pending-reboot":
		return format.Success(status)
	case "error":
		return format.Error(status)
	default:
		return status
	}
}

func formatBytes(bytesStr string) string {
	val, err := strconv.ParseInt(bytesStr, 10, 64)
	if err != nil || val == 0 {
		return "0 B"
	}
	switch {
	case val >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(val)/(1024*1024*1024))
	case val >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(val)/(1024*1024))
	case val >= 1024:
		return fmt.Sprintf("%.1f KB", float64(val)/1024)
	default:
		return fmt.Sprintf("%d B", val)
	}
}

func formatProgress(percent, downloaded, total string) string {
	pct := format.ParseInt(percent)
	text := fmt.Sprintf("%d%%", pct)
	if downloaded != "" && total != "" {
		text += fmt.Sprintf(" (%s / %s)", formatBytes(downloaded), formatBytes(total))
	}
	return text
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show OTA update status",
	Long:  `Display current OTA update status, installed version, and configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		settings, err := RedisClient.HGetAll("settings")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]any{
					"command": "ota-status",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get settings: %v\n"), err)
			}
			return
		}

		otaData, err := RedisClient.HGetAll("ota")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]any{
					"command": "ota-status",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get OTA status: %v\n"), err)
			}
			return
		}

		components := []string{"mdb", "dbc"}

		// Fetch installed versions from version:{component} hashes
		installedVersions := make(map[string]string)
		for _, component := range components {
			ver, err := RedisClient.HGet(fmt.Sprintf("version:%s", component), "version_id")
			if err == nil && ver != "" {
				installedVersions[component] = ver
			}
		}

		if JSONOutput != nil && *JSONOutput {
			result := make(map[string]map[string]any)

			for _, component := range components {
				c := make(map[string]any)

				if v, ok := installedVersions[component]; ok {
					c["installed-version"] = v
				} else {
					c["installed-version"] = nil
				}

				// Configuration from settings
				for _, key := range []string{"method", "channel", "check-interval", "last-check-time"} {
					settingKey := fmt.Sprintf("updates.%s.%s", component, key)
					if val, exists := settings[settingKey]; exists && val != "" {
						c[key] = val
					} else {
						c[key] = nil
					}
				}

				// Runtime status from ota hash
				for _, key := range []string{
					"status", "update-version", "update-method",
					"download-progress", "download-bytes", "download-total",
					"install-progress",
					"error", "error-message",
				} {
					otaKey := fmt.Sprintf("%s:%s", key, component)
					if val, exists := otaData[otaKey]; exists && val != "" {
						c[key] = val
					} else {
						c[key] = nil
					}
				}

				result[component] = c
			}

			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			format.PrintSection("OTA Update Status")
			fmt.Println()

			for _, component := range components {
				fmt.Printf("%s:\n", format.Info(component))

				// Installed version
				if v, ok := installedVersions[component]; ok {
					format.PrintKV("  installed", v)
				} else {
					format.PrintKV("  installed", format.Dim("unknown"))
				}

				// Status with color
				status := otaData[fmt.Sprintf("status:%s", component)]
				if status != "" {
					format.PrintKV("  status", colorizeOTAStatus(status))
				} else {
					format.PrintKV("  status", format.Dim("unknown"))
				}

				// Target version (only if not idle)
				if status != "" && status != "idle" {
					if ver := otaData[fmt.Sprintf("update-version:%s", component)]; ver != "" {
						format.PrintKV("  target", ver)
					}
				}

				// Update method
				if method := otaData[fmt.Sprintf("update-method:%s", component)]; method != "" {
					format.PrintKV("  method", method)
				}

				// Download progress (only during download)
				if status == "downloading" {
					progress := otaData[fmt.Sprintf("download-progress:%s", component)]
					downloaded := otaData[fmt.Sprintf("download-bytes:%s", component)]
					total := otaData[fmt.Sprintf("download-total:%s", component)]
					if progress != "" {
						format.PrintKV("  download", formatProgress(progress, downloaded, total))
					}
				}

				// Install progress (only during preparing/installing)
				if status == "preparing" || status == "installing" {
					if progress := otaData[fmt.Sprintf("install-progress:%s", component)]; progress != "" {
						format.PrintKV("  install", fmt.Sprintf("%s%%", progress))
					}
				}

				// Error info
				if status == "error" {
					if errType := otaData[fmt.Sprintf("error:%s", component)]; errType != "" {
						format.PrintKV("  error", format.Error(errType))
					}
					if errMsg := otaData[fmt.Sprintf("error-message:%s", component)]; errMsg != "" {
						format.PrintKV("  message", errMsg)
					}
				}

				// Configuration
				channel := settings[fmt.Sprintf("updates.%s.channel", component)]
				if channel != "" {
					format.PrintKV("  channel", channel)
				}
				interval := settings[fmt.Sprintf("updates.%s.check-interval", component)]
				if interval != "" {
					format.PrintKV("  check-interval", interval)
				}
				lastCheck := settings[fmt.Sprintf("updates.%s.last-check-time", component)]
				if lastCheck != "" {
					format.PrintKV("  last-check", lastCheck)
				}

				fmt.Println()
			}
		}
	},
}

func init() {
	OTACmd.AddCommand(statusCmd)
}
