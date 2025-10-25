package lsc

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var ledCmd = &cobra.Command{
	Use:   "led",
	Short: "Control LED indicators",
	Long:  `Control LED cues and fade animations.`,
}

var ledCueCmd = &cobra.Command{
	Use:   "cue <index>",
	Short: "Trigger LED cue",
	Long:  `Trigger a specific LED cue by index.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		indexStr := args[0]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-cue",
					"status":  "error",
					"error":   "invalid index: must be an integer",
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid index '%s': must be an integer\n"), indexStr)
			}
			return
		}

		if err := redisClient.LPush("scooter:led:cue", indexStr); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-cue",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send LED cue command: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "led-cue",
				"status":  "success",
				"index":   index,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s LED cue %d triggered\n", format.Success("✓"), index)
		}
	},
}

var ledFadeCmd = &cobra.Command{
	Use:   "fade <channel> <index>",
	Short: "Trigger LED fade animation",
	Long:  `Trigger a specific LED fade animation on a channel.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		channelStr := args[0]
		indexStr := args[1]

		channel, err := strconv.Atoi(channelStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-fade",
					"status":  "error",
					"error":   "invalid channel: must be an integer",
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid channel '%s': must be an integer\n"), channelStr)
			}
			return
		}

		index, err := strconv.Atoi(indexStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-fade",
					"status":  "error",
					"error":   "invalid index: must be an integer",
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid index '%s': must be an integer\n"), indexStr)
			}
			return
		}

		command := fmt.Sprintf("%d:%d", channel, index)
		if err := redisClient.LPush("scooter:led:fade", command); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-fade",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send LED fade command: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "led-fade",
				"status":  "success",
				"channel": channel,
				"index":   index,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s LED fade animation %d triggered on channel %d\n", format.Success("✓"), index, channel)
		}
	},
}

func init() {
	ledCmd.AddCommand(ledCueCmd)
	ledCmd.AddCommand(ledFadeCmd)
	rootCmd.AddCommand(ledCmd)
}
