package keycard

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <uid>",
	Short: "Remove a keycard UID from the authorized list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		uid := args[0]

		// Validate and normalize UID
		if err := validateUIDFormat(uid); err != nil {
			if *JSONOutput {
				printJSONResponse("error", nil, err)
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf("Error: Invalid UID: %v", err))
			}
			return err
		}
		uid = normalizeUID(uid)

		authorizedPath, _ := getKeycardPaths()

		// Read existing UIDs
		uids, err := readKeycardFile(authorizedPath)
		if err != nil {
			printError("Failed to read authorized UIDs", err)
			return err
		}

		// Find and remove UID
		found := false
		var newUIDs []string
		for _, existingUID := range uids {
			if existingUID == uid {
				found = true
			} else {
				newUIDs = append(newUIDs, existingUID)
			}
		}

		if !found {
			if *JSONOutput {
				printJSONResponse("error", nil, fmt.Errorf("UID not found"))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf("Error: UID %s not found", uid))
			}
			return fmt.Errorf("UID not found")
		}

		// Write updated UIDs
		if err := writeKeycardFile(authorizedPath, newUIDs); err != nil {
			printError("Failed to write authorized UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		if *JSONOutput {
			printJSONResponse("success", map[string]string{"uid": uid}, nil)
		} else {
			printSuccess(fmt.Sprintf("Removed keycard UID: %s", uid))
		}

		return nil
	},
}
