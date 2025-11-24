package keycard

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <uid>",
	Short: "Add a keycard UID to the authorized list",
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

		// Check if UID already exists
		for _, existingUID := range uids {
			if existingUID == uid {
				if *JSONOutput {
					printJSONResponse("error", nil, fmt.Errorf("UID already exists"))
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf("Error: UID %s already exists", uid))
				}
				return fmt.Errorf("UID already exists")
			}
		}

		// Add new UID
		uids = append(uids, uid)
		uids = removeDuplicates(uids)

		// Write updated UIDs
		if err := writeKeycardFile(authorizedPath, uids); err != nil {
			printError("Failed to write authorized UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		if *JSONOutput {
			printJSONResponse("success", map[string]string{"uid": uid}, nil)
		} else {
			printSuccess(fmt.Sprintf("Added keycard UID: %s", uid))
		}

		return nil
	},
}
