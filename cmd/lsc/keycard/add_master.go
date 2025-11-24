package keycard

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var addMasterCmd = &cobra.Command{
	Use:   "add-master <uid> [uid...]",
	Short: "Add one or more master keycard UIDs",
	Long:  `Add one or more master keycard UIDs for learn mode. Multiple UIDs can be provided as separate arguments.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, masterPath := getKeycardPaths()

		// Read existing master UIDs
		existingMasterUIDs, err := readKeycardFile(masterPath)
		if err != nil {
			printError("Failed to read master UIDs", err)
			return err
		}

		// Build set of existing UIDs for deduplication
		existingMap := make(map[string]bool)
		for _, uid := range existingMasterUIDs {
			existingMap[uid] = true
		}

		// Validate and add new UIDs
		var addedUIDs []string
		for _, uid := range args {
			if err := validateUIDFormat(uid); err != nil {
				if *JSONOutput {
					printJSONResponse("error", nil, err)
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf("Error: Invalid UID %s: %v", uid, err))
				}
				return err
			}
			normalizedUID := normalizeUID(uid)

			// Skip if already exists
			if existingMap[normalizedUID] {
				continue
			}

			addedUIDs = append(addedUIDs, normalizedUID)
		}

		// Combine existing and new
		allUIDs := append(existingMasterUIDs, addedUIDs...)
		allUIDs = removeDuplicates(allUIDs)

		// Write master UIDs
		if err := writeKeycardFile(masterPath, allUIDs); err != nil {
			printError("Failed to write master UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		if *JSONOutput {
			printJSONResponse("success", map[string]interface{}{"added": len(addedUIDs)}, nil)
		} else {
			if len(addedUIDs) == 0 {
				printSuccess("All UIDs already exist")
			} else {
				printSuccess(fmt.Sprintf("Added %d master keycard UID(s)", len(addedUIDs)))
			}
		}

		return nil
	},
}
