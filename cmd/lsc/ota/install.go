package ota

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <file-or-url>",
	Short: "Install OTA update",
	Long: `Install an OTA update from a local .mender file or download from URL.

This command will:
  - Download the file if a URL is provided
  - Install the update using mender-update
  - Report installation progress`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		source := args[0]
		var filePath string
		var err error

		// Check if source is a URL
		if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-install",
					"status":  "downloading",
					"url":     source,
				})
				fmt.Println(string(output))
			} else {
				fmt.Printf("Downloading update from %s...\n", source)
			}

			filePath, err = downloadFile(source)
			if err != nil {
				if JSONOutput != nil && *JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "ota-install",
						"status":  "error",
						"error":   fmt.Sprintf("download failed: %v", err),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to download update: %v\n"), err)
				}
				return
			}
			defer os.Remove(filePath) // Clean up downloaded file
		} else {
			filePath = source
		}

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-install",
					"status":  "error",
					"error":   fmt.Sprintf("file not found: %s", filePath),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("File not found: %s\n"), filePath)
			}
			return
		}

		// Install using mender-update
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-install",
				"status":  "installing",
				"file":    filePath,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("Installing update from %s...\n", filepath.Base(filePath))
		}

		menderCmd := exec.Command("mender-update", "install", filePath)
		menderCmd.Stdout = os.Stdout
		menderCmd.Stderr = os.Stderr

		if err := menderCmd.Run(); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-install",
					"status":  "error",
					"error":   fmt.Sprintf("installation failed: %v", err),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Installation failed: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-install",
				"status":  "success",
				"file":    filePath,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Update installed successfully"))
			fmt.Println(format.Warning("Note: A reboot may be required to complete the update"))
		}
	},
}

func downloadFile(url string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "mender-update-*.mender")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Download file
	resp, err := http.Get(url)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Copy to temp file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func init() {
	OTACmd.AddCommand(installCmd)
}
