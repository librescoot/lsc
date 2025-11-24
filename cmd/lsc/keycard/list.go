package keycard

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all authorized keycards",
	RunE: func(cmd *cobra.Command, args []string) error {
		authorizedPath, masterPath := getKeycardPaths()

		// Read authorized UIDs
		authorizedUIDs, err := readKeycardFile(authorizedPath)
		if err != nil {
			printError("Failed to read authorized UIDs", err)
			return err
		}

		// Read master UIDs
		masterUIDs, err := readKeycardFile(masterPath)
		if err != nil {
			printError("Failed to read master UIDs", err)
			return err
		}

		if *JSONOutput {
			response := map[string]interface{}{
				"authorized": formatUIDList(authorizedUIDs),
				"master":     formatUIDList(masterUIDs),
			}
			output, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(output))
		} else {
			if len(masterUIDs) > 0 {
				if len(masterUIDs) == 1 {
					fmt.Printf("Master Card: %s\n", formatUIDSpaceSeparated(masterUIDs[0]))
				} else {
					fmt.Printf("Master Cards (%d):\n", len(masterUIDs))
					for i, uid := range masterUIDs {
						fmt.Printf("  %d. %s\n", i+1, formatUIDSpaceSeparated(uid))
					}
				}
			}

			if len(authorizedUIDs) == 0 {
				fmt.Println("No authorized keycards")
			} else {
				fmt.Printf("\nAuthorized Cards (%d):\n", len(authorizedUIDs))
				for i, uid := range authorizedUIDs {
					fmt.Printf("  %d. %s\n", i+1, formatUIDSpaceSeparated(uid))
				}
			}
		}

		return nil
	},
}
