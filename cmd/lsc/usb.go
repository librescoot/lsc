package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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

var usbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current USB mode",
	Long:  `Display the current USB mode setting.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, err := redisClient.HGet("usb", "mode")
		if err != nil && redis.IsNil(err) {
			err = nil
			mode = ""
		}
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get USB mode: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if mode == "" {
			mode = "unknown"
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"mode": mode,
			})
			fmt.Println(string(output))
			return nil
		}

		format.PrintSection("USB Status")
		format.PrintKV("Mode", format.ColorizeState(mode))
		fmt.Println()
		return nil
	},
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
	usbCmd.AddCommand(usbStatusCmd)
	usbCmd.AddCommand(usbUmsCmd)
	usbCmd.AddCommand(usbNormalCmd)
	rootCmd.AddCommand(usbCmd)
	usbCmd.GroupID = "main"
}
