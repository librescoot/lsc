package format

import (
	"fmt"
	"strings"
)

// PrintSection prints a section header
func PrintSection(title string) {
	fmt.Printf("\n%s\n", Info("=== "+title+" ==="))
}

// PrintSubsection prints a subsection header
func PrintSubsection(title string) {
	fmt.Printf("\n%s\n", title)
}

// PrintKV prints a key-value pair
func PrintKV(key, value string) {
	fmt.Printf("%-20s %s\n", Dim(key+":"), value)
}

// PrintKVColored prints a key-value pair with colored value
func PrintKVColored(key, value string, colorFunc func(string) string) {
	fmt.Printf("%-20s %s\n", Dim(key+":"), colorFunc(value))
}

// PrintKeyValue prints a simple key: value line
func PrintKeyValue(key, value string) {
	fmt.Printf("%s: %s\n", key, value)
}

// SafeValue returns defaultVal if value is empty
func SafeValue(value, defaultVal string) string {
	if value == "" {
		return Dim(defaultVal)
	}
	return value
}

// SafeValueOr returns value or defaultVal with optional coloring
func SafeValueOr(value, defaultVal string) string {
	if value == "" || value == "0" || value == "unknown" {
		return Dim(defaultVal)
	}
	return value
}

// FormatPresence formats a presence boolean
func FormatPresence(present string) string {
	if present == "true" {
		return Success("Present")
	}
	return Dim("Not Present")
}

// FormatOnOff formats on/off states with colors
func FormatOnOff(state string) string {
	return ColorizeState(state)
}

// PrintTable prints a simple table
func PrintTable(headers []string, rows [][]string) {
	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers
	headerLine := ""
	separator := ""
	for i, header := range headers {
		headerLine += fmt.Sprintf("%-*s  ", widths[i], header)
		separator += strings.Repeat("-", widths[i]) + "  "
	}
	fmt.Println(Info(headerLine))
	fmt.Println(Dim(separator))

	// Print rows
	for _, row := range rows {
		line := ""
		for i, cell := range row {
			line += fmt.Sprintf("%-*s  ", widths[i], cell)
		}
		fmt.Println(line)
	}
}

// FormatList formats a list of items
func FormatList(items []string) {
	if len(items) == 0 {
		fmt.Println(Dim("  (none)"))
		return
	}
	for _, item := range items {
		fmt.Printf("  • %s\n", item)
	}
}

// FormatNotAvailable returns a dimmed "N/A" or custom message
func FormatNotAvailable(message string) string {
	if message == "" {
		return Dim("N/A")
	}
	return Dim(message)
}
