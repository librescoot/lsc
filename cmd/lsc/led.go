package lsc

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

// LED cue name to index mapping
var cueAliases = map[string]int{
	"all-off":                     0,
	"standby-to-parked-brake-off": 1,
	"standby-to-parked-brake-on":  2,
	"parked-to-drive":             3,
	"brake-off-to-brake-on":       4,
	"brake-on-to-brake-off":       5,
	"drive-to-parked":             6,
	"parked-brake-off-to-standby": 7,
	"parked-brake-on-to-standby":  8,
	"blink-none":                  9,
	"blink-left":                  10,
	"blink-right":                 11,
	"blink-both":                  12,
}

// LED channel name to index mapping
var channelAliases = map[string]int{
	"headlight":         0,
	"front-ring":        1,
	"brake":             2,
	"brake-light":       2,
	"blinker-front-left":  3,
	"blinker-left-front":  3,
	"blinker-front-right": 4,
	"blinker-right-front": 4,
	"number-plates":     5,
	"plates":            5,
	"blinker-rear-left": 6,
	"blinker-left-rear": 6,
	"blinker-rear-right": 7,
	"blinker-right-rear": 7,
}

// LED fade name to index mapping
var fadeAliases = map[string]int{
	"parking-smooth-on":  0,
	"smooth-off":         1,
	"brake-linear-on":    2,
	"brake-linear-off":   3,
	"brake-dim-on":       4,
	"brake-half-to-full": 5,
	"drive-light-on":     6,
	"brake-full-to-half": 7,
	"drive-light-off":    8,
	"brake-dim-off":      9,
	"blink":              10,
}

// parseCueIndex parses cue index from string (numeric or alias)
func parseCueIndex(s string) (int, error) {
	// Try numeric first
	if index, err := strconv.Atoi(s); err == nil {
		return index, nil
	}
	// Try alias lookup
	s = strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	if index, ok := cueAliases[s]; ok {
		return index, nil
	}
	return 0, fmt.Errorf("invalid cue '%s'", s)
}

// parseChannelIndex parses channel index from string (numeric or alias)
func parseChannelIndex(s string) (int, error) {
	// Try numeric first
	if index, err := strconv.Atoi(s); err == nil {
		return index, nil
	}
	// Try alias lookup
	s = strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	if index, ok := channelAliases[s]; ok {
		return index, nil
	}
	return 0, fmt.Errorf("invalid channel '%s'", s)
}

// parseFadeIndex parses fade index from string (numeric or alias)
func parseFadeIndex(s string) (int, error) {
	// Try numeric first
	if index, err := strconv.Atoi(s); err == nil {
		return index, nil
	}
	// Try alias lookup
	s = strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	if index, ok := fadeAliases[s]; ok {
		return index, nil
	}
	return 0, fmt.Errorf("invalid fade '%s'", s)
}

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
	Use:   "cue <index|name>",
	Short: "Trigger LED cue",
	Long: `Trigger a specific LED cue sequence.

Accepts either numeric index or text alias (case-insensitive, _ or - allowed).

Available cues:
  0  - all-off                        Turn off all LEDs
  1  - standby-to-parked-brake-off    Standby → Parked (brakes off)
  2  - standby-to-parked-brake-on     Standby → Parked (brakes on)
  3  - parked-to-drive                Parked → Ready to drive
  4  - brake-off-to-brake-on          Brake lights on
  5  - brake-on-to-brake-off          Brake lights off
  6  - drive-to-parked                Ready to drive → Parked
  7  - parked-brake-off-to-standby    Parked → Standby (brakes off)
  8  - parked-brake-on-to-standby     Parked → Standby (brakes on)
  9  - blink-none                     Turn off blinkers
  10 - blink-left                     Left blinker animation
  11 - blink-right                    Right blinker animation
  12 - blink-both                     Hazard lights (both blinkers)

Examples:
  lsc led cue 0               # Turn off all LEDs
  lsc led cue all-off         # Same using alias
  lsc led cue 10              # Activate left blinker
  lsc led cue blink-left      # Same using alias
  lsc led cue blink_both      # Hazard lights (underscores work too)`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		indexStr := args[0]
		index, err := parseCueIndex(indexStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-cue",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
			}
			return
		}

		if err := redisClient.LPush("scooter:led:cue", strconv.Itoa(index)); err != nil {
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
	Use:   "fade <channel> <index|name>",
	Short: "Trigger LED fade animation",
	Long: `Trigger a specific LED fade animation on a channel.

Accepts either numeric indices or text aliases (case-insensitive, _ or - allowed).

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
  0 - Headlight            (headlight)
  1 - Front ring           (front-ring)
  2 - Brake light          (brake, brake-light)
  3 - Blinker front left   (blinker-front-left)
  4 - Blinker front right  (blinker-front-right)
  5 - Number plates        (number-plates, plates)
  6 - Blinker rear left    (blinker-rear-left)
  7 - Blinker rear right   (blinker-rear-right)

Examples:
  lsc led fade 0 0                          # Smooth on headlight
  lsc led fade headlight parking-smooth-on  # Same using aliases
  lsc led fade 2 2                          # Fade on brake light
  lsc led fade brake brake-linear-on        # Same using aliases
  lsc led fade front-ring smooth-off        # Smooth off front ring`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		channelStr := args[0]
		indexStr := args[1]

		channel, err := parseChannelIndex(channelStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-fade",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
			}
			return
		}

		index, err := parseFadeIndex(indexStr)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "led-fade",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
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
