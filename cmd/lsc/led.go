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
	Long: `Control LED cues and fade animations.

LED Channels:
  0 - Headlight
  1 - Front ring
  2 - Brake light
  3 - Blinker front left
  4 - Blinker front right
  5 - Number plates
  6 - Blinker rear left
  7 - Blinker rear right`,
}

var ledCueCmd = &cobra.Command{
	Use:   "cue <index>",
	Short: "Trigger LED cue",
	Long: `Trigger a specific LED cue sequence.

Available cues:
  0  - all_off                        Turn off all LEDs
  1  - standby_to_parked_brake_off    Standby → Parked (brakes off)
  2  - standby_to_parked_brake_on     Standby → Parked (brakes on)
  3  - parked_to_drive                Parked → Ready to drive
  4  - brake_off_to_brake_on          Brake lights on
  5  - brake_on_to_brake_off          Brake lights off
  6  - drive_to_parked                Ready to drive → Parked
  7  - parked_brake_off_to_standby    Parked → Standby (brakes off)
  8  - parked_brake_on_to_standby     Parked → Standby (brakes on)
  9  - blink_none                     Turn off blinkers
  10 - blink_left                     Left blinker animation
  11 - blink_right                    Right blinker animation
  12 - blink_both                     Hazard lights (both blinkers)

Examples:
  lsc led cue 0     # Turn off all LEDs
  lsc led cue 10    # Activate left blinker
  lsc led cue 12    # Activate hazard lights`,
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
	Long: `Trigger a specific LED fade animation on a channel.

Available fade animations:
  0  - parking-smooth-on    Smooth fade on for parking lights
  1  - smooth-off           Smooth fade off
  2  - brake-linear-on      Linear fade on for brake light
  3  - brake-linear-off     Linear fade off for brake light
  4  - brake-dim-on         Dim brake light on
  5  - brake-half-to-full   Brake light half to full brightness
  6  - drive-light-on       Drive mode lights on
  7  - brake-full-to-half   Brake light full to half brightness
  8  - drive-light-off      Drive mode lights off
  9  - brake-dim-off        Dim brake light off
  10 - blink                Blink animation (for turn signals)

LED Channels:
  0 - Headlight
  1 - Front ring
  2 - Brake light
  3 - Blinker front left
  4 - Blinker front right
  5 - Number plates
  6 - Blinker rear left
  7 - Blinker rear right

Examples:
  lsc led fade 0 0      # Smooth on headlight
  lsc led fade 2 2      # Fade on brake light
  lsc led fade 1 1      # Smooth off front ring`,
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
