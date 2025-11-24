package keycard

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var removeMasterCmd = &cobra.Command{
	Use:   "remove-master <uid> [uid...]",
	Short: "Remove one or more master keycard UIDs",
	Long:  `Remove one or more master keycard UIDs. Multiple UIDs can be provided as separate arguments.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, masterPath := getKeycardPaths()

		// Read existing master UIDs
		existingMasterUIDs, err := readKeycardFile(masterPath)
		if err != nil {
			printError("Failed to read master UIDs", err)
			return err
		}

		// Validate all UIDs to remove
		var uidsToRemove []string
		for _, uid := range args {
			if err := validateUIDFormat(uid); err != nil {
				if *JSONOutput {
					printJSONResponse("error", nil, err)
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf("Error: Invalid UID %s: %v", uid, err))
				}
				return err
			}
			uidsToRemove = append(uidsToRemove, normalizeUID(uid))
		}

		// Build set for quick lookup
		toRemoveMap := make(map[string]bool)
		for _, uid := range uidsToRemove {
			toRemoveMap[uid] = true
		}

		// Remove UIDs
		var newUIDs []string
		var removedCount int
		for _, uid := range existingMasterUIDs {
			if toRemoveMap[uid] {
				removedCount++
			} else {
				newUIDs = append(newUIDs, uid)
			}
		}

		// Check if any were removed
		if removedCount == 0 {
			if *JSONOutput {
				printJSONResponse("error", nil, fmt.Errorf("no UIDs found to remove"))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", "Error: No UIDs found to remove")
			}
			return fmt.Errorf("no UIDs found to remove")
		}

		// Write updated master UIDs
		if err := writeKeycardFile(masterPath, newUIDs); err != nil {
			printError("Failed to write master UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		if *JSONOutput {
			printJSONResponse("success", map[string]interface{}{"removed": removedCount}, nil)
		} else {
			printSuccess(fmt.Sprintf("Removed %d master keycard UID(s)", removedCount))
		}

		return nil
	},
}
