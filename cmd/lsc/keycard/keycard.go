package keycard

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool
)

var KeycardCmd = &cobra.Command{
	Use:   "keycard",
	Short: "Manage keycard authentication",
	Long:  `Manage authorized keycards for the scooter.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to list when called without subcommand
		return listCmd.RunE(cmd, args)
	},
}

// SetRedisClient sets the Redis client for all keycard commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for all keycard commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// getKeycardPaths returns the paths for authorized UIDs and master UID files
func getKeycardPaths() (authorizedPath, masterPath string) {
	return "/data/keycard/authorized_uids.txt", "/data/keycard/master_uids.txt"
}

// readKeycardFile reads and parses UIDs from a file
// Normalizes UIDs to concatenated uppercase hex format (e.g., "042A3D6A0D6580")
func readKeycardFile(path string) ([]string, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []string{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var uids []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			// Normalize to concatenated uppercase format for internal comparison
			normalized := normalizeUID(line)
			uids = append(uids, normalized)
		}
	}

	return uids, nil
}

// writeKeycardFile writes UIDs to a file, creating directories as needed
// Writes in space-separated uppercase hex format (e.g., "04 05 B5 82 D0 1E 91")
func writeKeycardFile(path string, uids []string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Filter out empty UIDs and format as space-separated hex bytes
	var formattedUIDs []string
	for _, uid := range uids {
		if strings.TrimSpace(uid) != "" {
			formatted := formatUIDSpaceSeparated(uid)
			formattedUIDs = append(formattedUIDs, formatted)
		}
	}

	// Write UIDs to file
	content := strings.Join(formattedUIDs, "\n")
	if content != "" {
		content += "\n"
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// writeKeycardExportFile writes authorized and master UIDs to a file in section-based format
// Format:
// [authorized]
// 04 05 B5 82 D0 1E 91
// FA F2 38 A5
//
// [master]
// 04 2A 3D 6A 0D 65 80
func writeKeycardExportFile(path string, authorizedUIDs, masterUIDs []string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var content strings.Builder

	// Write authorized section
	if len(authorizedUIDs) > 0 {
		content.WriteString("[authorized]\n")
		for _, uid := range authorizedUIDs {
			if strings.TrimSpace(uid) != "" {
				formatted := formatUIDSpaceSeparated(uid)
				content.WriteString(formatted + "\n")
			}
		}
	}

	// Write master section
	if len(masterUIDs) > 0 {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString("[master]\n")
		for _, uid := range masterUIDs {
			if strings.TrimSpace(uid) != "" {
				formatted := formatUIDSpaceSeparated(uid)
				content.WriteString(formatted + "\n")
			}
		}
	}

	if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// validateUIDFormat validates that a UID is a valid hex string
// Accepts hex strings up to 10 bytes (20 hex chars, minimum 1 byte / 2 hex chars)
func validateUIDFormat(uid string) error {
	// Remove common separators
	uid = strings.ReplaceAll(uid, ":", "")
	uid = strings.ReplaceAll(uid, "-", "")
	uid = strings.ReplaceAll(uid, " ", "")
	uid = strings.ToUpper(uid)

	// Check length (max 10 bytes = 20 hex chars, min 1 byte = 2 hex chars)
	if len(uid) < 2 || len(uid) > 20 {
		return fmt.Errorf("UID must be 1-10 bytes in hex format (2-20 characters)")
	}

	// Check if all characters are valid hex
	for _, char := range uid {
		if !((char >= '0' && char <= '9') || (char >= 'A' && char <= 'F')) {
			return fmt.Errorf("UID must contain only hexadecimal characters")
		}
	}

	return nil
}

// normalizeUID normalizes a UID by removing separators and converting to uppercase
func normalizeUID(uid string) string {
	uid = strings.ReplaceAll(uid, ":", "")
	uid = strings.ReplaceAll(uid, "-", "")
	uid = strings.ReplaceAll(uid, " ", "")
	return strings.ToUpper(uid)
}

// formatUIDSpaceSeparated formats a UID as space-separated uppercase hex bytes
// Input can be: "04A1B2C3D4E5F6", "04:A1:B2:C3:D4:E5:F6", "04 A1 B2 C3 D4 E5 F6"
// Output: "04 A1 B2 C3 D4 E5 F6"
func formatUIDSpaceSeparated(uid string) string {
	// Normalize first (remove all separators, uppercase)
	uid = normalizeUID(uid)

	// Split into pairs of hex characters
	var pairs []string
	for i := 0; i < len(uid); i += 2 {
		if i+1 < len(uid) {
			pairs = append(pairs, uid[i:i+2])
		} else if i < len(uid) {
			pairs = append(pairs, uid[i:])
		}
	}

	return strings.Join(pairs, " ")
}

// formatUIDAList formats a list of normalized UIDs as space-separated format for display
func formatUIDList(uids []string) []string {
	var formatted []string
	for _, uid := range uids {
		formatted = append(formatted, formatUIDSpaceSeparated(uid))
	}
	return formatted
}

// restartKeycardService restarts the librescoot-keycard service
func restartKeycardService() error {
	// Try to restart the service
	cmd := exec.Command("systemctl", "restart", "librescoot-keycard")
	if err := cmd.Run(); err != nil {
		// Service might not exist or permission denied - non-fatal
		return nil
	}
	return nil
}

// removeDuplicates removes duplicate UIDs from a list
func removeDuplicates(uids []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, uid := range uids {
		if !seen[uid] {
			seen[uid] = true
			result = append(result, uid)
		}
	}
	sort.Strings(result)
	return result
}

// printJSONResponse prints a JSON response
func printJSONResponse(status string, data interface{}, err error) {
	response := map[string]interface{}{
		"command": "keycard",
		"status":  status,
	}
	if data != nil {
		response["data"] = data
	}
	if err != nil {
		response["error"] = err.Error()
	}
	output, _ := json.MarshalIndent(response, "", "  ")
	fmt.Println(string(output))
}

// printError prints an error message
func printError(msg string, err error) {
	if *JSONOutput {
		printJSONResponse("error", nil, fmt.Errorf("%s: %w", msg, err))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", format.Error(fmt.Sprintf("Error: %s: %v", msg, err)))
	}
}

// printSuccess prints a success message
func printSuccess(msg string) {
	if *JSONOutput {
		printJSONResponse("success", nil, nil)
	} else {
		fmt.Println(format.Success(msg))
	}
}

func init() {
	KeycardCmd.AddCommand(listCmd)
	KeycardCmd.AddCommand(addCmd)
	KeycardCmd.AddCommand(removeCmd)
	KeycardCmd.AddCommand(addMasterCmd)
	KeycardCmd.AddCommand(removeMasterCmd)
	KeycardCmd.AddCommand(importCmd)
	KeycardCmd.AddCommand(exportCmd)
}
