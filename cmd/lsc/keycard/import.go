package keycard

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import keycards from a file",
	Long: `Import keycards from a file. The file should contain one UID per line.
Lines starting with # are treated as comments. Empty lines are ignored.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			if *JSONOutput {
				printJSONResponse("error", nil, fmt.Errorf("failed to read file: %w", err))
			} else {
				fmt.Fprintf(os.Stderr, "Error: Failed to read file: %v\n", err)
			}
			return err
		}

		// Parse UIDs
		var importedUIDs []string
		var invalidUIDs []string
		lines := strings.Split(string(data), "\n")
		for lineNum, line := range lines {
			line = strings.TrimSpace(line)
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Validate and normalize
			if err := validateUIDFormat(line); err != nil {
				invalidUIDs = append(invalidUIDs, fmt.Sprintf("Line %d: %s (%v)", lineNum+1, line, err))
				continue
			}
			importedUIDs = append(importedUIDs, normalizeUID(line))
		}

		// Remove duplicates
		importedUIDs = removeDuplicates(importedUIDs)

		// Get current authorized UIDs
		authorizedPath, _ := getKeycardPaths()
		existingUIDs, err := readKeycardFile(authorizedPath)
		if err != nil {
			printError("Failed to read authorized UIDs", err)
			return err
		}

		// Check for conflicts
		existingMap := make(map[string]bool)
		for _, uid := range existingUIDs {
			existingMap[uid] = true
		}

		var conflictUIDs []string
		for _, uid := range importedUIDs {
			if existingMap[uid] {
				conflictUIDs = append(conflictUIDs, uid)
			}
		}

		// Add imported UIDs to existing
		allUIDs := append(existingUIDs, importedUIDs...)
		allUIDs = removeDuplicates(allUIDs)

		// Write updated UIDs
		if err := writeKeycardFile(authorizedPath, allUIDs); err != nil {
			printError("Failed to write authorized UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		if *JSONOutput {
			response := map[string]interface{}{
				"imported":  len(importedUIDs),
				"conflicts": len(conflictUIDs),
				"invalid":   len(invalidUIDs),
			}
			if len(invalidUIDs) > 0 {
				response["invalid_lines"] = invalidUIDs
			}
			if len(conflictUIDs) > 0 {
				response["conflict_uids"] = conflictUIDs
			}
			output, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(output))
		} else {
			fmt.Printf("Imported %d keycards\n", len(importedUIDs))
			if len(conflictUIDs) > 0 {
				fmt.Printf("Skipped %d existing keycards\n", len(conflictUIDs))
			}
			if len(invalidUIDs) > 0 {
				fmt.Printf("Warning: %d invalid lines:\n", len(invalidUIDs))
				for _, invalid := range invalidUIDs {
					fmt.Printf("  %s\n", invalid)
				}
			}
		}

		return nil
	},
}
