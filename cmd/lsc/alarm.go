package lsc

import (
	"context"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/internal/confirm"
	"librescoot/lsc/internal/format"

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
	Run: func(cmd *cobra.Command, args []string) {
		// Get alarm status
		status, err := redisClient.HGet("alarm", "status")
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to get alarm status: %v\n"), err)
			return
		}

		// Get alarm settings
		enabled, _ := redisClient.HGet("settings", "alarm.enabled")
		honk, _ := redisClient.HGet("settings", "alarm.honk")
		duration, _ := redisClient.HGet("settings", "alarm.duration")

		format.PrintSection("Alarm Status")
		format.PrintKV("Status", format.ColorizeState(status))
		format.PrintKV("Enabled", format.ColorizeState(enabled))
		format.PrintKV("Honk", format.SafeValueOr(honk, "false"))
		format.PrintKV("Duration", format.SafeValueOr(duration, "10")+" seconds")
		fmt.Println()
	},
}

var alarmArmCmd = &cobra.Command{
	Use:   "arm",
	Short: "Arm the alarm",
	Long:  `Enable the alarm system. Will arm when vehicle enters stand-by state.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Arming alarm...")

		// Set alarm.enabled to true
		if err := redisClient.HSet("settings", "alarm.enabled", "true"); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to enable alarm: %v\n"), err)
			return
		}

		// Publish the change
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", "alarm.enabled"); err != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Alarm enabled but publish failed: %v\n"), err)
			return
		}

		if noBlock {
			fmt.Println(format.Success("Alarm enabled"))
			return
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
			fmt.Println(format.Success(fmt.Sprintf("Alarm %s", status)))
			return
		}

		for {
			select {
			case <-timeout:
				fmt.Println(format.Success("Alarm enabled (will arm when vehicle enters stand-by)"))
				return
			case msg := <-ch:
				if msg.Payload == "status" {
					status, _ := redisClient.HGet("alarm", "status")
					if status == "armed" || status == "delay-armed" {
						fmt.Println(format.Success(fmt.Sprintf("Alarm %s", status)))
						return
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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Disarming alarm...")

		// Set alarm.enabled to false
		if err := redisClient.HSet("settings", "alarm.enabled", "false"); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to disable alarm: %v\n"), err)
			return
		}

		// Publish the change
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", "alarm.enabled"); err != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Alarm disabled but publish failed: %v\n"), err)
			return
		}

		if noBlock {
			fmt.Println(format.Success("Alarm disabled"))
			return
		}

		// Wait for alarm status to change to disarmed
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx2, redisClient, "alarm", "status", "disarmed", 5*time.Second); err != nil {
			fmt.Println(format.Success("Alarm disabled"))
			return
		}

		fmt.Println(format.Success("Alarm disarmed"))
	},
}

var alarmTriggerCmd = &cobra.Command{
	Use:   "trigger [duration]",
	Short: "Manually trigger the alarm",
	Long:  `Manually trigger the alarm for a specified duration (in seconds). Uses alarm.duration setting if not specified.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Get duration from args or settings
		duration := "10"
		if len(args) > 0 {
			duration = args[0]
		} else {
			if d, err := redisClient.HGet("settings", "alarm.duration"); err == nil && d != "" {
				duration = d
			}
		}

		fmt.Printf("Triggering alarm for %s seconds...\n", duration)

		// Send trigger command
		command := fmt.Sprintf("start:%s", duration)
		if err := redisClient.LPush("scooter:alarm", command); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to trigger alarm: %v\n"), err)
			return
		}

		fmt.Println(format.Success("Alarm triggered"))
	},
}

func init() {
	alarmCmd.PersistentFlags().BoolVar(&noBlock, "no-block", false, "Don't wait for status confirmation")

	alarmCmd.AddCommand(alarmStatusCmd)
	alarmCmd.AddCommand(alarmArmCmd)
	alarmCmd.AddCommand(alarmDisarmCmd)
	alarmCmd.AddCommand(alarmTriggerCmd)
	rootCmd.AddCommand(alarmCmd)
}
