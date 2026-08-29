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
Lines starting with # are treated as comments. Empty lines are ignored.
A [master] section marks the UIDs that follow as master cards; UIDs before
any section header, or under [authorized], are imported as regular cards.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			printError("Failed to read file", err)
			return err
		}

		var authorized, masters, invalid []string
		section := ""
		for lineNum, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.ToLower(strings.Trim(line, "[]"))
				continue
			}
			if err := validateUIDFormat(line); err != nil {
				invalid = append(invalid, fmt.Sprintf("Line %d: %s (%v)", lineNum+1, line, err))
				continue
			}
			if section == "master" {
				masters = append(masters, normalizeUID(line))
			} else {
				authorized = append(authorized, normalizeUID(line))
			}
		}
		authorized = removeDuplicates(authorized)
		masters = removeDuplicates(masters)

		useService := serviceRunning()
		if !useService {
			fallbackNotice()
		}

		// Every UID goes in one at a time so keycard-service applies its own
		// duplicate and role rules to each, and publishes an event for each.
		importAuthorized, authorizedConflicts, err := importUIDs(useService, authorized, "add:", authorizedFilePath(),
			"already-authorized", "already-registered")
		if err != nil {
			printError("Failed to import authorized keycards", err)
			return err
		}
		importMasters, masterConflicts, err := importUIDs(useService, masters, "master:add:", masterFilePath(),
			"already-registered")
		if err != nil {
			printError("Failed to import master keycards", err)
			return err
		}

		totalImported := importAuthorized + importMasters
		totalConflicts := authorizedConflicts + masterConflicts

		if *JSONOutput {
			response := map[string]interface{}{
				"imported":             totalImported,
				"authorized_imported":  importAuthorized,
				"master_imported":      importMasters,
				"conflicts":            totalConflicts,
				"authorized_conflicts": authorizedConflicts,
				"master_conflicts":     masterConflicts,
				"invalid":              len(invalid),
			}
			if len(invalid) > 0 {
				response["invalid_lines"] = invalid
			}
			output, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		fmt.Printf("Imported %d keycards (%d authorized, %d master)\n", totalImported, importAuthorized, importMasters)
		if totalConflicts > 0 {
			fmt.Printf("Skipped %d existing keycards (%d authorized, %d master)\n", totalConflicts, authorizedConflicts, masterConflicts)
		}
		if len(invalid) > 0 {
			fmt.Printf("Warning: %d invalid lines:\n", len(invalid))
			for _, line := range invalid {
				fmt.Printf("  %s\n", line)
			}
		}
		return nil
	},
}

// importUIDs adds every UID and reports how many landed and how many were
// already registered. skipCodes are the replies that mean "already there".
func importUIDs(useService bool, uids []string, prefix, path string, skipCodes ...string) (imported, conflicts int, err error) {
	for _, uid := range uids {
		if useService {
			skipped, err := isResult(prefix+uid, skipCodes...)
			if err != nil {
				return imported, conflicts, err
			}
			if skipped {
				conflicts++
			} else {
				imported++
			}
			continue
		}

		if err := addUIDToFile(path, uid); err != nil {
			conflicts++
			continue
		}
		imported++
	}
	return imported, conflicts, nil
}
