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

		// Parse UIDs from section-based format
		var importedAuthorizedUIDs []string
		var importedMasterUIDs []string
		var invalidUIDs []string

		lines := strings.Split(string(data), "\n")
		currentSection := ""

		for lineNum, line := range lines {
			line = strings.TrimSpace(line)

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Check for section headers
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				currentSection = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
				continue
			}

			// Validate and normalize
			if err := validateUIDFormat(line); err != nil {
				invalidUIDs = append(invalidUIDs, fmt.Sprintf("Line %d: %s (%v)", lineNum+1, line, err))
				continue
			}

			normalizedUID := normalizeUID(line)

			// Add to appropriate section (default to authorized if no section)
			if currentSection == "master" {
				importedMasterUIDs = append(importedMasterUIDs, normalizedUID)
			} else {
				importedAuthorizedUIDs = append(importedAuthorizedUIDs, normalizedUID)
			}
		}

		// Remove duplicates
		importedAuthorizedUIDs = removeDuplicates(importedAuthorizedUIDs)
		importedMasterUIDs = removeDuplicates(importedMasterUIDs)

		// Get current authorized and master UIDs
		authorizedPath, masterPath := getKeycardPaths()
		existingAuthorizedUIDs, err := readKeycardFile(authorizedPath)
		if err != nil {
			printError("Failed to read authorized UIDs", err)
			return err
		}

		existingMasterUIDs, err := readKeycardFile(masterPath)
		if err != nil {
			printError("Failed to read master UIDs", err)
			return err
		}

		// Check for conflicts in authorized UIDs
		existingAuthorizedMap := make(map[string]bool)
		for _, uid := range existingAuthorizedUIDs {
			existingAuthorizedMap[uid] = true
		}

		var authorizedConflictCount int
		var newAuthorizedUIDs []string
		for _, uid := range importedAuthorizedUIDs {
			if existingAuthorizedMap[uid] {
				authorizedConflictCount++
			} else {
				newAuthorizedUIDs = append(newAuthorizedUIDs, uid)
			}
		}

		// Check for conflicts in master UIDs
		existingMasterMap := make(map[string]bool)
		for _, uid := range existingMasterUIDs {
			existingMasterMap[uid] = true
		}

		var masterConflictCount int
		var newMasterUIDs []string
		for _, uid := range importedMasterUIDs {
			if existingMasterMap[uid] {
				masterConflictCount++
			} else {
				newMasterUIDs = append(newMasterUIDs, uid)
			}
		}

		// Combine and write
		allAuthorizedUIDs := append(existingAuthorizedUIDs, newAuthorizedUIDs...)
		allAuthorizedUIDs = removeDuplicates(allAuthorizedUIDs)

		allMasterUIDs := append(existingMasterUIDs, newMasterUIDs...)
		allMasterUIDs = removeDuplicates(allMasterUIDs)

		if err := writeKeycardFile(authorizedPath, allAuthorizedUIDs); err != nil {
			printError("Failed to write authorized UIDs", err)
			return err
		}

		if err := writeKeycardFile(masterPath, allMasterUIDs); err != nil {
			printError("Failed to write master UIDs", err)
			return err
		}

		// Restart keycard service
		restartKeycardService()

		totalImported := len(newAuthorizedUIDs) + len(newMasterUIDs)
		totalConflicts := authorizedConflictCount + masterConflictCount

		if *JSONOutput {
			response := map[string]interface{}{
				"imported":            totalImported,
				"authorized_imported": len(newAuthorizedUIDs),
				"master_imported":     len(newMasterUIDs),
				"conflicts":           totalConflicts,
				"authorized_conflicts": authorizedConflictCount,
				"master_conflicts":    masterConflictCount,
				"invalid":             len(invalidUIDs),
			}
			if len(invalidUIDs) > 0 {
				response["invalid_lines"] = invalidUIDs
			}
			output, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(output))
		} else {
			fmt.Printf("Imported %d keycards (%d authorized, %d master)\n", totalImported, len(newAuthorizedUIDs), len(newMasterUIDs))
			if totalConflicts > 0 {
				fmt.Printf("Skipped %d existing keycards (%d authorized, %d master)\n", totalConflicts, authorizedConflictCount, masterConflictCount)
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
