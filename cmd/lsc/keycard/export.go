package keycard

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export keycards to a file",
	Long:  `Export all authorized keycards and master keycards to a file. The output is plain text with one UID per line.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Reads go to the UID files, not to the service's list command; see
		// getKeycardPaths for why.
		authorizedPath, masterPath := getKeycardPaths()

		// Read authorized UIDs
		authorizedUIDs, err := readKeycardFile(authorizedPath)
		if err != nil {
			printError("Failed to read authorized UIDs", err)
			return err
		}

		// Read master UIDs
		rawMasterUIDs, err := readKeycardFile(masterPath)
		if err != nil {
			printError("Failed to read master UIDs", err)
			return err
		}
		// The NONE sentinel is service state, not a card, so it is not
		// exported: importing it back would try to add it as a UID.
		masterUIDs, _ := splitMasters(rawMasterUIDs)

		// Write to file in section-based format
		if err := writeKeycardExportFile(filePath, authorizedUIDs, masterUIDs); err != nil {
			if *JSONOutput {
				printJSONResponse("error", nil, fmt.Errorf("failed to write file: %w", err))
			} else {
				fmt.Fprintf(os.Stderr, "Error: Failed to write file: %v\n", err)
			}
			return err
		}

		totalCount := len(authorizedUIDs) + len(masterUIDs)

		if *JSONOutput {
			response := map[string]interface{}{
				"file":             filePath,
				"exported":         totalCount,
				"authorized_count": len(authorizedUIDs),
				"master_count":     len(masterUIDs),
			}
			output, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(output))
		} else {
			fmt.Printf("Exported %d keycards (%d authorized, %d master) to %s\n",
				totalCount, len(authorizedUIDs), len(masterUIDs), filePath)
		}

		return nil
	},
}
