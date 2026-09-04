package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var usbCmd = &cobra.Command{
	Use:   "usb",
	Short: "Control USB mode",
	Long:  `Control USB mode switching between UMS (USB Mass Storage) and normal (network) mode.`,
}

var (
	usbStatusLogLines int
	usbLogLines       int
)

var usbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show USB mode and UMS cycle state",
	Long: `Display the USB mode plus the state of the current or last UMS cycle:
the processing status and step, and how the last reboot phase ended.

Pass --log to also print the tail of the per-cycle log.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fields, err := redisClient.HGetAll("usb")
		if err != nil && redis.IsNil(err) {
			err = nil
			fields = nil
		}
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get USB status: %v\n"), err)
			}
			return cli.ErrSilent
		}

		var logEntries []string
		if usbStatusLogLines > 0 {
			logEntries, err = readUsbLog(usbStatusLogLines)
			if err != nil {
				fmt.Fprintf(os.Stderr, format.Warning("Failed to read USB log: %v\n"), err)
			}
		}

		mode := fields["mode"]
		if mode == "" {
			mode = "unknown"
		}
		status := fields["status"]
		if status == "" {
			status = "unknown"
		}

		if JSONOutput {
			out := map[string]interface{}{
				"mode":               mode,
				"status":             status,
				"step":               fields["step"],
				"detail":             fields["detail"],
				"progress":           fields["progress"],
				"last_result":        fields["last-result"],
				"last_result_detail": fields["last-result-detail"],
				"last_result_time":   fields["last-result-time"],
			}
			if usbStatusLogLines > 0 {
				out["log"] = logEntries
			}
			output, _ := json.Marshal(out)
			fmt.Println(string(output))
			return nil
		}

		format.PrintSection("USB Status")
		format.PrintKV("Mode", format.ColorizeState(mode))
		format.PrintKV("Status", format.ColorizeState(status))
		if step := fields["step"]; step != "" {
			format.PrintKV("Step", describeStep(step))
		}
		if detail := fields["detail"]; detail != "" {
			format.PrintKV("Detail", detail)
		}
		if progress := fields["progress"]; progress != "" && progress != "0" {
			format.PrintKV("Progress", progress+"%")
		}
		if result := fields["last-result"]; result != "" {
			format.PrintKV("Last result", colorizeUsbResult(result))
			if d := fields["last-result-detail"]; d != "" {
				fmt.Printf("%*s%s\n", kvIndent, "", d)
			}
			if t := fields["last-result-time"]; t != "" {
				fmt.Printf("%*s%s\n", kvIndent, "", format.Dim(t))
			}
		}
		fmt.Println()

		if usbStatusLogLines > 0 {
			printUsbLog(logEntries)
		}
		return nil
	},
}

var usbLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the log of the current or last UMS cycle",
	Long: `Print entries from the usb:log list, which ums-service writes as it
processes the drive. The list is cleared when a new UMS session starts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := readUsbLog(usbLogLines)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to read USB log: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			if entries == nil {
				entries = []string{}
			}
			output, _ := json.Marshal(map[string]interface{}{
				"log": entries,
			})
			fmt.Println(string(output))
			return nil
		}

		printUsbLog(entries)
		return nil
	},
}

const kvIndent = 21

// readUsbLog reverses Redis's newest-first entries for display.
func readUsbLog(n int) ([]string, error) {
	if n <= 0 {
		n = 20
	}
	entries, err := redisClient.LRange("usb:log", 0, int64(n-1))
	if err != nil {
		if redis.IsNil(err) {
			return nil, nil
		}
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

func printUsbLog(entries []string) {
	format.PrintSection("USB Log")
	if len(entries) == 0 {
		fmt.Println(format.Dim("(empty)"))
		fmt.Println()
		return
	}
	for _, e := range entries {
		if strings.Contains(e, "ERROR:") {
			fmt.Println(format.Error(e))
		} else {
			fmt.Println(e)
		}
	}
	fmt.Println()
}

func describeStep(step string) string {
	rest, ok := strings.CutPrefix(step, "waiting-")
	if !ok {
		return step
	}
	if rest == "vehicle-state" {
		return format.Warning(step) + format.Dim("  (installs done, waiting for a state that allows a reboot)")
	}
	components := strings.Split(rest, "+")
	for i, c := range components {
		components[i] = strings.ToUpper(c)
	}
	return format.Warning(step) + format.Dim("  (installing on "+strings.Join(components, " and ")+")")
}

func colorizeUsbResult(result string) string {
	switch result {
	case "reboot-triggered":
		return format.Success(result)
	case "vehicle-state":
		return format.Warning(result)
	case "timeout", "install-error", "error":
		return format.Error(result)
	default:
		return result
	}
}

var usbUmsCmd = &cobra.Command{
	Use:   "ums",
	Short: "Switch to UMS (USB Mass Storage) mode",
	Long:  `Switch USB to Mass Storage mode. The device will appear as a USB drive when connected.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !JSONOutput {
			fmt.Println("Switching to UMS mode...")
		}

		if err := redisClient.HSet("usb", "mode", "ums"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ums",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to set USB mode: %v\n"), err)
			}
			return cli.ErrSilent
		}

		ctx := context.Background()
		if err := redisClient.Publish(ctx, "usb", "mode"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ums",
					"status":  "warning",
					"message": "Mode set but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Mode set but publish failed: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ums",
				"status":  "success",
				"mode":    "ums",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("USB mode set to UMS (Mass Storage)"))
		}
		return nil
	},
}

var usbNormalCmd = &cobra.Command{
	Use:   "normal",
	Short: "Switch to normal (network) mode",
	Long:  `Switch USB to normal network mode.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !JSONOutput {
			fmt.Println("Switching to normal mode...")
		}

		if err := redisClient.HSet("usb", "mode", "normal"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "normal",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to set USB mode: %v\n"), err)
			}
			return cli.ErrSilent
		}

		ctx := context.Background()
		if err := redisClient.Publish(ctx, "usb", "mode"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "normal",
					"status":  "warning",
					"message": "Mode set but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Mode set but publish failed: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "normal",
				"status":  "success",
				"mode":    "normal",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("USB mode set to normal (network)"))
		}
		return nil
	},
}

func init() {
	usbStatusCmd.Flags().IntVar(&usbStatusLogLines, "log", 0, "Also print the last N entries of the UMS cycle log")
	usbStatusCmd.Flags().Lookup("log").NoOptDefVal = "20"
	usbLogCmd.Flags().IntVarP(&usbLogLines, "lines", "n", 20, "Number of log entries to show")

	usbCmd.AddCommand(usbStatusCmd)
	usbCmd.AddCommand(usbLogCmd)
	usbCmd.AddCommand(usbUmsCmd)
	usbCmd.AddCommand(usbNormalCmd)
	rootCmd.AddCommand(usbCmd)
	usbCmd.GroupID = "main"
}
