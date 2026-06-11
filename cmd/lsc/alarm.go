package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/confirm"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var alarmCmd = &cobra.Command{
	Use:   "alarm",
	Short: "Control alarm system",
	Long:  `Control the motion-based alarm system.`,
}

var alarmStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show alarm status",
	Long:  `Display current alarm status and settings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get alarm status
		status, err := redisClient.HGet("alarm", "status")
		if err != nil && redis.IsNil(err) {
			err = nil
			status = ""
		}
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get alarm status: %v\n"), err)
			}
			return cli.ErrSilent
		}

		// Get alarm settings
		enabled, _ := redisClient.HGet("settings", "alarm.enabled")
		honk, _ := redisClient.HGet("settings", "alarm.honk")
		duration, _ := redisClient.HGet("settings", "alarm.duration")
		seatboxTrigger, _ := redisClient.HGet("settings", "alarm.seatbox-trigger")
		hairTrigger, _ := redisClient.HGet("settings", "alarm.hairtrigger")
		hairTriggerDuration, _ := redisClient.HGet("settings", "alarm.hairtrigger-duration")

		if JSONOutput {
			// Parse duration or use the settings schema default
			durationVal := duration
			if durationVal == "" {
				durationVal = "60"
			}

			output, _ := json.Marshal(map[string]interface{}{
				"status":                status,
				"enabled":               enabled == "true",
				"honk":                  honk == "true",
				"duration":              durationVal,
				"seatbox_trigger":       seatboxTrigger != "false",
				"hair_trigger":          hairTrigger == "true",
				"hair_trigger_duration": format.SafeValueOr(hairTriggerDuration, "3"),
			})
			fmt.Println(string(output))
			return nil
		}

		format.PrintSection("Alarm Status")
		format.PrintKV("Status", format.ColorizeState(status))
		format.PrintKV("Enabled", format.ColorizeState(enabled))
		format.PrintKV("Honk", format.SafeValueOr(honk, "false"))
		format.PrintKV("Duration", format.SafeValueOr(duration, "60")+" seconds")
		format.PrintKV("Seatbox Trigger", format.SafeValueOr(seatboxTrigger, "true"))
		format.PrintKV("Hair Trigger", format.SafeValueOr(hairTrigger, "false"))
		format.PrintKV("Hair Trigger Duration", format.SafeValueOr(hairTriggerDuration, "3")+" seconds")
		fmt.Println()
		return nil
	},
}

var alarmArmCmd = &cobra.Command{
	Use:   "arm",
	Short: "Arm the alarm",
	Long:  `Enable the alarm system. Will arm when vehicle enters stand-by state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !JSONOutput {
			fmt.Println("Arming alarm...")
		}

		// Set alarm.enabled to true
		if err := redisClient.HSet("settings", "alarm.enabled", "true"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "arm",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to enable alarm: %v\n"), err)
			}
			return cli.ErrSilent
		}

		// Publish the change
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", "alarm.enabled"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "arm",
					"status":  "warning",
					"message": "Alarm enabled but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Alarm enabled but publish failed: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if noBlock {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "arm",
					"status":  "enabled",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Alarm enabled"))
			}
			return nil
		}

		// Wait for alarm to arm (if vehicle is in stand-by)
		ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pubsub := redisClient.Subscribe(ctx2, "alarm")
		defer pubsub.Close()

		ch := pubsub.Channel()
		timeout := time.After(10 * time.Second)

		// Check current status immediately
		status, _ := redisClient.HGet("alarm", "status")
		if status == "armed" || status == "delay-armed" {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command":      "arm",
					"status":       "success",
					"alarm_status": status,
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success(fmt.Sprintf("Alarm %s", status)))
			}
			return nil
		}

		for {
			select {
			case <-timeout:
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "arm",
						"status":  "enabled",
						"message": "Will arm when vehicle enters stand-by",
					})
					fmt.Println(string(output))
				} else {
					fmt.Println(format.Success("Alarm enabled (will arm when vehicle enters stand-by)"))
				}
				return nil
			case msg := <-ch:
				if msg.Payload == "status" {
					status, _ := redisClient.HGet("alarm", "status")
					if status == "armed" || status == "delay-armed" {
						if JSONOutput {
							output, _ := json.Marshal(map[string]interface{}{
								"command":      "arm",
								"status":       "success",
								"alarm_status": status,
							})
							fmt.Println(string(output))
						} else {
							fmt.Println(format.Success(fmt.Sprintf("Alarm %s", status)))
						}
						return nil
					}
				}
			}
		}
	},
}

var alarmDisarmCmd = &cobra.Command{
	Use:   "disarm",
	Short: "Disarm the alarm",
	Long:  `Disable the alarm system.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !JSONOutput {
			fmt.Println("Disarming alarm...")
		}

		// Set alarm.enabled to false
		if err := redisClient.HSet("settings", "alarm.enabled", "false"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "disarm",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to disable alarm: %v\n"), err)
			}
			return cli.ErrSilent
		}

		// Publish the change
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", "alarm.enabled"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "disarm",
					"status":  "warning",
					"message": "Alarm disabled but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Alarm disabled but publish failed: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if noBlock {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "disarm",
					"status":  "disabled",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Alarm disabled"))
			}
			return nil
		}

		// Wait for alarm status to change to disarmed
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx2, redisClient, "alarm", "status", "disarmed", 5*time.Second); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "disarm",
					"status":  "disabled",
					"message": "alarm.enabled set to false but status confirmation timed out",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Warning("Alarm disabled (status confirmation timed out)"))
			}
			return nil
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":      "disarm",
				"status":       "success",
				"alarm_status": "disarmed",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Alarm disarmed"))
		}
		return nil
	},
}

var alarmTriggerCmd = &cobra.Command{
	Use:   "trigger [duration]",
	Short: "Manually trigger the alarm",
	Long:  `Manually trigger the alarm for a specified duration (in seconds). Uses alarm.duration setting if not specified.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get duration from args or settings
		duration := "10"
		if len(args) > 0 {
			duration = args[0]
			if d, err := strconv.Atoi(duration); err != nil || d <= 0 {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "trigger",
						"status":  "error",
						"error":   fmt.Sprintf("invalid duration '%s': must be a positive number of seconds", duration),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Invalid duration '%s': must be a positive number of seconds\n"), duration)
				}
				return cli.ErrSilent
			}
		} else {
			if d, err := redisClient.HGet("settings", "alarm.duration"); err == nil && d != "" {
				duration = d
			}
		}

		if !JSONOutput {
			fmt.Printf("Triggering alarm for %s seconds...\n", duration)
		}

		// Send trigger command
		command := fmt.Sprintf("start:%s", duration)
		if err := redisClient.LPush("scooter:alarm", command); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command":  "trigger",
					"status":   "error",
					"error":    err.Error(),
					"duration": duration,
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to trigger alarm: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":  "trigger",
				"status":   "success",
				"duration": duration,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Alarm triggered"))
		}
		return nil
	},
}

var alarmSilenceCmd = &cobra.Command{
	Use:   "silence",
	Short: "Temporarily silence the alarm",
	Long: `Temporarily disarm the alarm without changing the alarm.enabled setting.
The alarm will re-arm automatically when the vehicle next enters stand-by.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !JSONOutput {
			fmt.Println("Silencing alarm...")
		}

		if err := redisClient.LPush("scooter:alarm", "disarm"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "silence",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to silence alarm: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "silence",
				"status":  "success",
				"message": "Alarm silenced (will re-arm on next stand-by)",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Alarm silenced (will re-arm on next stand-by)"))
		}
		return nil
	},
}

func init() {
	alarmCmd.PersistentFlags().BoolVar(&noBlock, "no-block", false, "Don't wait for status confirmation")

	alarmCmd.AddCommand(alarmStatusCmd)
	alarmCmd.AddCommand(alarmArmCmd)
	alarmCmd.AddCommand(alarmDisarmCmd)
	alarmCmd.AddCommand(alarmSilenceCmd)
	alarmCmd.AddCommand(alarmTriggerCmd)
	rootCmd.AddCommand(alarmCmd)
	alarmCmd.GroupID = "main"
}
