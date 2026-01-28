package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var validChannels = []string{"stable", "testing", "nightly"}

func isValidChannel(channel string) bool {
	for _, c := range validChannels {
		if c == channel {
			return true
		}
	}
	return false
}

var channelCmd = &cobra.Command{
	Use:   "channel [channel]",
	Short: "Get or set update channel for MDB and DBC",
	Long: `Get or set the release channel for OTA updates.

Without arguments, displays the current channels for MDB and DBC.
With a channel argument, sets both MDB and DBC to the specified channel.

Valid channels:
  stable   - Production releases only
  testing  - Beta/release candidate versions
  nightly  - Latest development builds

Examples:
  lsc ota channel           # Show current channels
  lsc ota channel stable    # Set both to stable
  lsc ota channel nightly   # Set both to nightly`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return showChannels()
		}
		return setChannels(args[0])
	},
}

func showChannels() error {
	settings, err := RedisClient.HGetAll("settings")
	if err != nil {
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-channel",
				"status":  "error",
				"error":   err.Error(),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprintf(os.Stderr, format.Error("Failed to get settings: %v\n"), err)
		}
		return err
	}

	mdbChannel := settings["updates.mdb.channel"]
	dbcChannel := settings["updates.dbc.channel"]

	if JSONOutput != nil && *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command": "ota-channel",
			"status":  "success",
			"mdb":     mdbChannel,
			"dbc":     dbcChannel,
		})
		fmt.Println(string(output))
	} else {
		format.PrintSection("Update Channels")
		if mdbChannel == "" {
			format.PrintKV("MDB", format.Dim("(not set)"))
		} else {
			format.PrintKV("MDB", mdbChannel)
		}
		if dbcChannel == "" {
			format.PrintKV("DBC", format.Dim("(not set)"))
		} else {
			format.PrintKV("DBC", dbcChannel)
		}
	}
	return nil
}

func setChannels(channel string) error {
	if !isValidChannel(channel) {
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-channel",
				"status":  "error",
				"error":   fmt.Sprintf("Invalid channel '%s'. Must be one of: stable, testing, nightly", channel),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprintf(os.Stderr, format.Error("Invalid channel '%s'. Must be one of: stable, testing, nightly\n"), channel)
		}
		return fmt.Errorf("invalid channel '%s'", channel)
	}

	// Set MDB channel
	if err := RedisClient.HSet("settings", "updates.mdb.channel", channel); err != nil {
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-channel",
				"status":  "error",
				"error":   fmt.Sprintf("Failed to set MDB channel: %v", err),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprintf(os.Stderr, format.Error("Failed to set MDB channel: %v\n"), err)
		}
		return err
	}

	// Set DBC channel
	if err := RedisClient.HSet("settings", "updates.dbc.channel", channel); err != nil {
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-channel",
				"status":  "error",
				"error":   fmt.Sprintf("Failed to set DBC channel: %v", err),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprintf(os.Stderr, format.Error("Failed to set DBC channel: %v\n"), err)
		}
		return err
	}

	// Publish settings change
	ctx := context.Background()
	if err := RedisClient.Publish(ctx, "settings", "updates.mdb.channel"); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Channel set but failed to notify services for MDB: %v\n"), err)
	}
	if err := RedisClient.Publish(ctx, "settings", "updates.dbc.channel"); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Channel set but failed to notify services for DBC: %v\n"), err)
	}

	if JSONOutput != nil && *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command": "ota-channel",
			"status":  "success",
			"channel": channel,
			"message": fmt.Sprintf("Update channel set to '%s' for MDB and DBC", channel),
		})
		fmt.Println(string(output))
	} else {
		fmt.Println(format.Success(fmt.Sprintf("Update channel set to '%s' for MDB and DBC", channel)))
	}
	return nil
}

func init() {
	OTACmd.AddCommand(channelCmd)
}
