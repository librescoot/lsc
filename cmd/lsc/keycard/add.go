package keycard

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <uid>",
	Short: "Add a keycard UID to the authorized list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Local format check only, so an obvious typo fails without a round
		// trip. Duplicate and role rules belong to keycard-service.
		if err := validateUIDFormat(args[0]); err != nil {
			printError("Invalid UID", err)
			return err
		}
		uid := normalizeUID(args[0])

		if serviceRunning() {
			if err := runKeycardCommand("add:" + uid); err != nil {
				printError("Failed to add keycard", err)
				return err
			}
		} else {
			fallbackNotice()
			if err := addUIDToFile(authorizedFilePath(), uid); err != nil {
				printError("Failed to add keycard", err)
				return err
			}
		}

		if *JSONOutput {
			printJSONResponse("success", map[string]string{"uid": uid}, nil)
		} else {
			printSuccess(fmt.Sprintf("Added keycard UID: %s", formatUIDSpaceSeparated(uid)))
		}
		return nil
	},
}
