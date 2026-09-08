package keycard

import (
	"fmt"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:               "remove <uid>",
	Short:             "Remove a keycard UID from the authorized list",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAuthorizedUIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateUIDFormat(args[0]); err != nil {
			printError("Invalid UID", err)
			return err
		}
		uid := normalizeUID(args[0])

		if serviceRunning() {
			// keycard-service owns the anti-lockout rule and answers
			// error:last-credential when this would strand the vehicle.
			if err := runKeycardCommand("remove:" + uid); err != nil {
				printError("Failed to remove keycard", err)
				return err
			}
		} else {
			fallbackNotice()
			if err := removeUIDFromFile(authorizedFilePath(), uid, true); err != nil {
				printError("Failed to remove keycard", err)
				return err
			}
		}

		if *JSONOutput {
			printJSONResponse("success", map[string]string{"uid": uid}, nil)
		} else {
			printSuccess(fmt.Sprintf("Removed keycard UID: %s", formatUIDSpaceSeparated(uid)))
		}
		return nil
	},
}
