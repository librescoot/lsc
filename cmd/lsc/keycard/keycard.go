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
		return listCmd.RunE(cmd, args)
	},
}

func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

func getKeycardPaths() (authorizedPath, masterPath string) {
	return "/data/keycard/authorized_uids.txt", "/data/keycard/master_uids.txt"
}

// UID files accept separators but normalize to uppercase contiguous hex internally.
func readKeycardFile(path string) ([]string, error) {
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
		if line != "" && !strings.HasPrefix(line, "#") {
			normalized := normalizeUID(line)
			uids = append(uids, normalized)
		}
	}

	return uids, nil
}

// The on-device format is one uppercase, space-separated UID per line.
func writeKeycardFile(path string, uids []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var formattedUIDs []string
	for _, uid := range uids {
		if strings.TrimSpace(uid) != "" {
			formatted := formatUIDSpaceSeparated(uid)
			formattedUIDs = append(formattedUIDs, formatted)
		}
	}

	content := strings.Join(formattedUIDs, "\n")
	if content != "" {
		content += "\n"
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Exports preserve authorization class with [authorized] and [master] sections.
func writeKeycardExportFile(path string, authorizedUIDs, masterUIDs []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var content strings.Builder

	if len(authorizedUIDs) > 0 {
		content.WriteString("[authorized]\n")
		for _, uid := range authorizedUIDs {
			if strings.TrimSpace(uid) != "" {
				formatted := formatUIDSpaceSeparated(uid)
				content.WriteString(formatted + "\n")
			}
		}
	}

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

// NFC UIDs are one to ten bytes (two to twenty hexadecimal characters).
func validateUIDFormat(uid string) error {
	uid = strings.ReplaceAll(uid, ":", "")
	uid = strings.ReplaceAll(uid, "-", "")
	uid = strings.ReplaceAll(uid, " ", "")
	uid = strings.ToUpper(uid)

	if len(uid) < 2 || len(uid) > 20 {
		return fmt.Errorf("UID must be 1-10 bytes in hex format (2-20 characters)")
	}

	for _, char := range uid {
		if !((char >= '0' && char <= '9') || (char >= 'A' && char <= 'F')) {
			return fmt.Errorf("UID must contain only hexadecimal characters")
		}
	}

	return nil
}

func normalizeUID(uid string) string {
	uid = strings.ReplaceAll(uid, ":", "")
	uid = strings.ReplaceAll(uid, "-", "")
	uid = strings.ReplaceAll(uid, " ", "")
	return strings.ToUpper(uid)
}

// formatUIDSpaceSeparated accepts normalized or separator-delimited input.
func formatUIDSpaceSeparated(uid string) string {
	uid = normalizeUID(uid)

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

func formatUIDList(uids []string) []string {
	var formatted []string
	for _, uid := range uids {
		formatted = append(formatted, formatUIDSpaceSeparated(uid))
	}
	return formatted
}

// restartKeycardService restarts the librescoot-keycard service. A failure
// here (service missing, permission denied) is non-fatal: the UID file has
// already been written, and the running service will pick it up on its own
// next restart or reload.
func restartKeycardService() {
	cmd := exec.Command("systemctl", "restart", "librescoot-keycard")
	_ = cmd.Run()
}

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

func printError(msg string, err error) {
	if *JSONOutput {
		printJSONResponse("error", nil, fmt.Errorf("%s: %w", msg, err))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", format.Error(fmt.Sprintf("Error: %s: %v", msg, err)))
	}
}

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
