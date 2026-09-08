package keycard

import (
	"encoding/json"
	"fmt"
	"os"
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

func completeUIDFile(path string, args []string, toComplete string, masters bool) ([]string, cobra.ShellCompDirective) {
	uids, err := readKeycardFile(path)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if masters {
		uids, _ = splitMasters(uids)
	}
	used := make(map[string]bool, len(args))
	for _, uid := range args {
		used[normalizeUID(uid)] = true
	}
	candidates := make([]string, 0, len(uids))
	prefix := normalizeUID(toComplete)
	for _, uid := range uids {
		uid = normalizeUID(uid)
		if !used[uid] && strings.HasPrefix(uid, prefix) {
			candidates = append(candidates, uid)
		}
	}
	sort.Strings(candidates)
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func completeAuthorizedUIDs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeUIDFile(authorizedFilePath(), args, toComplete, false)
}

func completeMasterUIDs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeUIDFile(masterFilePath(), args, toComplete, true)
}

// Reads go to the files, not the service: command-result has no request
// correlation, so a multi-entry reply cannot be collected reliably. Mutations
// go through the command interface, see service.go.
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

// Fallback path only. Bare uppercase hex, the shape the service writes.
func writeKeycardFile(path string, uids []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var formattedUIDs []string
	for _, uid := range uids {
		if strings.TrimSpace(uid) != "" {
			formattedUIDs = append(formattedUIDs, normalizeUID(uid))
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

// masterDisabled records that no physical master is wanted; unlike an empty
// list it stops bootstrap re-arming on the next start.
const masterDisabled = "NONE"

// splitMasters separates the real master UIDs from the sentinel.
func splitMasters(uids []string) (real []string, disabled bool) {
	for _, uid := range uids {
		if uid == masterDisabled {
			disabled = true
			continue
		}
		real = append(real, uid)
	}
	return real, disabled
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
