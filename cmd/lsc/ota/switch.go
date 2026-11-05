package ota

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:       "switch <stable|testing|nightly>",
	Short:     "Request a reboot-safe channel switch for both boards",
	Args:      cobra.ExactArgs(1),
	ValidArgs: validChannels,
	RunE: func(cmd *cobra.Command, args []string) error {
		channel := args[0]
		if !isValidChannel(channel) {
			return fmt.Errorf("invalid channel %q", channel)
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return fmt.Errorf("generate request ID: %w", err)
		}
		id := hex.EncodeToString(token[:])
		if err := RedisClient.LPush("scooter:update:mdb", "switch-channel:"+channel+":"+id); err != nil {
			return err
		}
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-deadline.C:
				return fmt.Errorf("switch request not confirmed; check OTA status before retrying")
			case <-ticker.C:
				result, err := RedisClient.HGet("ota", "switch-result")
				if err != nil || !strings.HasPrefix(result, id+":") {
					continue
				}
				state := strings.TrimPrefix(result, id+":")
				if state != "accepted" && state != "complete" && state != "partial" {
					return fmt.Errorf("switch request %s", state)
				}
				if JSONOutput != nil && *JSONOutput {
					data, _ := json.Marshal(map[string]string{"command": "ota-switch", "status": state, "channel": channel, "request_id": id})
					fmt.Println(string(data))
				} else {
					fmt.Printf("Channel switch to %s accepted (request %s)\n", channel, id)
				}
				return nil
			}
		}
	},
}

func init() { OTACmd.AddCommand(switchCmd) }
