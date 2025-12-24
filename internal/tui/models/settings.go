package models

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/registry"
	"librescoot/lsc/internal/tui/styles"
)

type editMode int

const (
	modeNavigate editMode = iota
	modeEditText
	modeEditSelect
	modeEditLocation
)

type locationEditField int

const (
	fieldLabel locationEditField = iota
	fieldLatitude
	fieldLongitude
)

type savedLocation struct {
	index       int
	label       string
	latitude    string
	longitude   string
	createdAt   string
	lastUsedAt  string
}

type Settings struct {
	redisClient     *redis.Client
	settings        []registry.Setting
	cursor          int
	values          map[string]string
	mode            editMode
	editBuffer      string
	selectCursor    int // For selection mode
	width           int
	height          int
	errorMsg        string
	successMsg      string
	quitting        bool
	// Location editing state
	editLocationIdx   int
	editLocationLabel string
	editLocationLat   string
	editLocationLon   string
	editLocationField locationEditField
}

func NewSettings(redisClient *redis.Client) Settings {
	return Settings{
		redisClient: redisClient,
		settings:    registry.Settings,
		cursor:      0,
		values:      make(map[string]string),
		mode:        modeNavigate,
	}
}

func (m Settings) Init() tea.Cmd {
	return tea.Batch(
		tea.ClearScreen,
		m.fetchSettings,
	)
}

func (m Settings) fetchSettings() tea.Msg {
	values, err := m.redisClient.HGetAll("settings")
	if err != nil {
		return errMsg{err}
	}
	return settingsLoadedMsg{values}
}

type settingsLoadedMsg struct {
	values map[string]string
}

type settingUpdatedMsg struct {
	key   string
	value string
}

type errMsg struct {
	err error
}

func (m Settings) parseSavedLocations() []savedLocation {
	locationMap := make(map[int]*savedLocation)

	for key, value := range m.values {
		if !strings.HasPrefix(key, "dashboard.saved-locations.") {
			continue
		}

		// Parse: dashboard.saved-locations.N.field
		parts := strings.Split(key, ".")
		if len(parts) != 4 {
			continue
		}

		index := 0
		fmt.Sscanf(parts[2], "%d", &index)
		field := parts[3]

		if locationMap[index] == nil {
			locationMap[index] = &savedLocation{index: index}
		}

		switch field {
		case "label":
			locationMap[index].label = value
		case "latitude":
			locationMap[index].latitude = value
		case "longitude":
			locationMap[index].longitude = value
		case "created-at":
			locationMap[index].createdAt = value
		case "last-used-at":
			locationMap[index].lastUsedAt = value
		}
	}

	// Convert map to sorted slice
	var locations []savedLocation
	for i := 0; i < 100; i++ { // Max 100 locations
		if loc, exists := locationMap[i]; exists {
			locations = append(locations, *loc)
		}
	}

	return locations
}

// getCurrentItem returns either a setting or a location index based on cursor position
func (m Settings) getCurrentItem() (setting *registry.Setting, locationIndex int, isLocation bool) {
	// Build category groups the same way as View() to ensure consistent indexing
	categoryGroups := make(map[string][]registry.Setting)
	for _, setting := range m.settings {
		if strings.HasPrefix(setting.Key, "dashboard.saved-locations.") {
			continue
		}
		categoryGroups[setting.Category] = append(categoryGroups[setting.Category], setting)
	}

	categories := registry.GetCategories()
	currentIndex := 0

	for _, category := range categories {
		categorySettings := categoryGroups[category]
		if len(categorySettings) == 0 && category != "Saved Locations" {
			continue
		}

		if category == "Saved Locations" {
			locations := m.parseSavedLocations()
			if currentIndex <= m.cursor && m.cursor < currentIndex+len(locations) {
				loc := locations[m.cursor-currentIndex]
				return nil, loc.index, true
			}
			currentIndex += len(locations)
		} else {
			// Iterate through category settings in same order as View()
			for i := range categorySettings {
				if currentIndex == m.cursor {
					return &categorySettings[i], 0, false
				}
				currentIndex++
			}
		}
	}

	return nil, 0, false
}

func (m Settings) saveSettings(key, value string) tea.Cmd {
	return func() tea.Msg {
		// Validate
		if err := registry.ValidateValue(key, value); err != nil {
			return errMsg{fmt.Errorf("validation failed: %v", err)}
		}

		// Set in Redis
		if err := m.redisClient.HSet("settings", key, value); err != nil {
			return errMsg{fmt.Errorf("failed to save: %v", err)}
		}

		// Publish change
		ctx := context.Background()
		if err := m.redisClient.Publish(ctx, "settings", key); err != nil {
			return errMsg{fmt.Errorf("saved but publish failed: %v", err)}
		}

		return settingUpdatedMsg{key: key, value: value}
	}
}

type settingDeletedMsg struct {
	key string
}

func (m Settings) unsetSetting(key string) tea.Cmd {
	return func() tea.Msg {
		// Delete from Redis
		if err := m.redisClient.HDel("settings", key); err != nil {
			return errMsg{fmt.Errorf("failed to delete: %v", err)}
		}

		// Publish change
		ctx := context.Background()
		if err := m.redisClient.Publish(ctx, "settings", key); err != nil {
			return errMsg{fmt.Errorf("deleted but publish failed: %v", err)}
		}

		return settingDeletedMsg{key: key}
	}
}

func (m Settings) deleteLocation(index int) tea.Cmd {
	return func() tea.Msg {
		// Delete all fields for this location
		fields := []string{
			fmt.Sprintf("dashboard.saved-locations.%d.label", index),
			fmt.Sprintf("dashboard.saved-locations.%d.latitude", index),
			fmt.Sprintf("dashboard.saved-locations.%d.longitude", index),
			fmt.Sprintf("dashboard.saved-locations.%d.created-at", index),
			fmt.Sprintf("dashboard.saved-locations.%d.last-used-at", index),
		}

		for _, field := range fields {
			if err := m.redisClient.HDel("settings", field); err != nil {
				return errMsg{fmt.Errorf("failed to delete location: %v", err)}
			}
		}

		// Publish change for first field (services will reload all)
		ctx := context.Background()
		if err := m.redisClient.Publish(ctx, "settings", fields[0]); err != nil {
			return errMsg{fmt.Errorf("deleted but publish failed: %v", err)}
		}

		return settingDeletedMsg{key: fmt.Sprintf("location-%d", index)}
	}
}

func (m Settings) saveLocation(index int, label, latitude, longitude string) tea.Cmd {
	return func() tea.Msg {
		// Validate latitude and longitude
		if latitude != "" {
			if _, err := strconv.ParseFloat(latitude, 64); err != nil {
				return errMsg{fmt.Errorf("invalid latitude: must be a number")}
			}
		}
		if longitude != "" {
			if _, err := strconv.ParseFloat(longitude, 64); err != nil {
				return errMsg{fmt.Errorf("invalid longitude: must be a number")}
			}
		}

		// Save all fields
		fields := map[string]string{
			fmt.Sprintf("dashboard.saved-locations.%d.label", index):     label,
			fmt.Sprintf("dashboard.saved-locations.%d.latitude", index):  latitude,
			fmt.Sprintf("dashboard.saved-locations.%d.longitude", index): longitude,
		}

		for key, value := range fields {
			if err := m.redisClient.HSet("settings", key, value); err != nil {
				return errMsg{fmt.Errorf("failed to save location: %v", err)}
			}
		}

		// Publish change
		ctx := context.Background()
		firstKey := fmt.Sprintf("dashboard.saved-locations.%d.label", index)
		if err := m.redisClient.Publish(ctx, "settings", firstKey); err != nil {
			return errMsg{fmt.Errorf("saved but publish failed: %v", err)}
		}

		return settingUpdatedMsg{key: fmt.Sprintf("location-%d", index), value: label}
	}
}

func (m Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Clear messages after any action
	oldMode := m.mode

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case settingsLoadedMsg:
		m.values = msg.values
		return m, nil

	case settingUpdatedMsg:
		m.values[msg.key] = msg.value
		m.successMsg = fmt.Sprintf("✓ Saved %s = %s", msg.key, msg.value)
		m.errorMsg = ""
		m.mode = modeNavigate
		m.selectCursor = 0
		return m, nil

	case settingDeletedMsg:
		delete(m.values, msg.key)
		m.successMsg = fmt.Sprintf("✓ Deleted %s", msg.key)
		m.errorMsg = ""
		return m, nil

	case errMsg:
		m.errorMsg = msg.err.Error()
		m.successMsg = ""
		m.mode = modeNavigate
		m.selectCursor = 0
		return m, nil

	case tea.KeyMsg:
		// Clear messages on next key press if not in edit mode
		if oldMode == modeNavigate {
			m.errorMsg = ""
			m.successMsg = ""
		}

		switch m.mode {
		case modeNavigate:
			switch msg.String() {
			case "q":
				m.quitting = true
				return m, tea.Quit

			case "esc":
				// Return to main menu
				menu := NewMenu("", m.redisClient)
				return menu, menu.Init()

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}

			case "down", "j":
				// Count total visible items (settings + locations)
				totalItems := 0
				categories := registry.GetCategories()
				for _, category := range categories {
					if category == "Saved Locations" {
						totalItems += len(m.parseSavedLocations())
					} else {
						for _, s := range m.settings {
							if s.Category == category && !strings.HasPrefix(s.Key, "dashboard.saved-locations.") {
								totalItems++
							}
						}
					}
				}
				if m.cursor < totalItems-1 {
					m.cursor++
				}

			case "enter", "e":
				// Enter edit mode
				setting, locationIdx, isLocation := m.getCurrentItem()
				if isLocation {
					// Enter location edit mode
					locations := m.parseSavedLocations()
					for _, loc := range locations {
						if loc.index == locationIdx {
							m.mode = modeEditLocation
							m.editLocationIdx = loc.index
							m.editLocationLabel = loc.label
							m.editLocationLat = loc.latitude
							m.editLocationLon = loc.longitude
							m.editLocationField = fieldLabel
							m.errorMsg = ""
							m.successMsg = ""
							break
						}
					}
				} else if setting != nil {
					m.errorMsg = ""
					m.successMsg = ""

					// If setting has possible values, use selection mode
					if len(setting.PossibleValues) > 0 {
						m.mode = modeEditSelect
						// Find current value in list
						m.selectCursor = 0
						currentValue := m.values[setting.Key]
						for i, val := range setting.PossibleValues {
							if val == currentValue {
								m.selectCursor = i
								break
							}
						}
					} else {
						// Otherwise use text input
						m.mode = modeEditText
						m.editBuffer = m.values[setting.Key]
					}
				}

			case "d", "x":
				// Unset/delete setting or location
				setting, locationIdx, isLocation := m.getCurrentItem()
				if isLocation {
					return m, m.deleteLocation(locationIdx)
				} else if setting != nil {
					return m, m.unsetSetting(setting.Key)
				}
			}

	case modeEditSelect:
			setting, _, isLocation := m.getCurrentItem()
			if !isLocation && setting != nil {
				switch msg.String() {
				case "esc":
					m.mode = modeNavigate
					m.selectCursor = 0

				case "up", "k":
					if m.selectCursor > 0 {
						m.selectCursor--
					}

				case "down", "j":
					if m.selectCursor < len(setting.PossibleValues)-1 {
						m.selectCursor++
					}

				case "enter":
					// Save selected value
					selectedValue := setting.PossibleValues[m.selectCursor]
					return m, m.saveSettings(setting.Key, selectedValue)
				}
			}

		case modeEditText:
			switch msg.Type {
			case tea.KeyEsc:
				// Cancel editing
				m.mode = modeNavigate
				m.editBuffer = ""
				m.errorMsg = ""

			case tea.KeyEnter:
				// Save value
				setting, _, isLocation := m.getCurrentItem()
				if !isLocation && setting != nil {
					return m, m.saveSettings(setting.Key, m.editBuffer)
				}

			case tea.KeyBackspace:
				if len(m.editBuffer) > 0 {
					m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
				}

			case tea.KeySpace:
				m.editBuffer += " "

			case tea.KeyRunes:
				// Add typed characters
				m.editBuffer += string(msg.Runes)
			}

		case modeEditLocation:
			switch msg.Type {
			case tea.KeyEsc:
				// Cancel editing
				m.mode = modeNavigate
				m.errorMsg = ""

			case tea.KeyTab:
				// Move to next field
				m.editLocationField = (m.editLocationField + 1) % 3

			case tea.KeyShiftTab:
				// Move to previous field
				if m.editLocationField == 0 {
					m.editLocationField = 2
				} else {
					m.editLocationField--
				}

			case tea.KeyEnter:
				// Save all location fields
				return m, m.saveLocation(m.editLocationIdx, m.editLocationLabel, m.editLocationLat, m.editLocationLon)

			case tea.KeyBackspace:
				// Delete character from current field
				switch m.editLocationField {
				case fieldLabel:
					if len(m.editLocationLabel) > 0 {
						m.editLocationLabel = m.editLocationLabel[:len(m.editLocationLabel)-1]
					}
				case fieldLatitude:
					if len(m.editLocationLat) > 0 {
						m.editLocationLat = m.editLocationLat[:len(m.editLocationLat)-1]
					}
				case fieldLongitude:
					if len(m.editLocationLon) > 0 {
						m.editLocationLon = m.editLocationLon[:len(m.editLocationLon)-1]
					}
				}

			case tea.KeySpace:
				// Add space to current field (only label allows spaces)
				if m.editLocationField == fieldLabel {
					m.editLocationLabel += " "
				}

			case tea.KeyRunes:
				// Add typed characters to current field
				switch m.editLocationField {
				case fieldLabel:
					m.editLocationLabel += string(msg.Runes)
				case fieldLatitude:
					m.editLocationLat += string(msg.Runes)
				case fieldLongitude:
					m.editLocationLon += string(msg.Runes)
				}
			}
		}
	}

	return m, nil
}

func (m Settings) View() string {
	if m.quitting {
		return ""
	}

	var s string

	// Title
	s = styles.Title.Render("Settings Browser")
	s += "\n\n"

	// Group settings by category (exclude individual location fields)
	categoryGroups := make(map[string][]registry.Setting)
	categories := registry.GetCategories()
	for _, setting := range m.settings {
		// Skip individual saved location fields - we'll display them grouped
		if strings.HasPrefix(setting.Key, "dashboard.saved-locations.") {
			continue
		}
		categoryGroups[setting.Category] = append(categoryGroups[setting.Category], setting)
	}

	// Display all settings without complex scrolling for now
	currentIndex := 0

	for _, category := range categories {
		// Category header
		categorySettings := categoryGroups[category]
		// Don't skip Saved Locations even if empty (handled specially)
		if len(categorySettings) == 0 && category != "Saved Locations" {
			continue
		}

		s += styles.Subtitle.Render(category) + "\n"

		// Special handling for Saved Locations
		if category == "Saved Locations" {
			locations := m.parseSavedLocations()
			if len(locations) == 0 {
				s += styles.Help.Render("    (no saved locations)") + "\n"
			} else {
				for _, loc := range locations {
					cursor := "    "
					if currentIndex == m.cursor {
						cursor = "  ▸ "
					}

					// Format location as: Label (lat, lon)
					displayText := fmt.Sprintf("Location %d", loc.index)
					if loc.label != "" {
						displayText = loc.label
					}
					if loc.latitude != "" && loc.longitude != "" {
						displayText += fmt.Sprintf(" (%s, %s)", loc.latitude, loc.longitude)
					}

					if currentIndex == m.cursor {
						displayText = styles.Selected.Render(displayText)
					}

					s += fmt.Sprintf("%s%s\n", cursor, displayText)

					// Show details for selected location
					if currentIndex == m.cursor {
						details := fmt.Sprintf("Index: %d", loc.index)
						if loc.createdAt != "" {
							details += fmt.Sprintf(" • Created: %s", loc.createdAt)
						}
						if loc.lastUsedAt != "" {
							details += fmt.Sprintf(" • Last used: %s", loc.lastUsedAt)
						}
						s += fmt.Sprintf("      %s\n", styles.Help.Render(details))
					}

					currentIndex++
				}
			}
		} else {
			// Normal setting display for other categories
			for _, setting := range categorySettings {
				cursor := "    "
				if currentIndex == m.cursor {
					cursor = "  ▸ "
				}

				// Get current value
				value := m.values[setting.Key]
				if value == "" {
					value = styles.Help.Render("(not set)")
				}

				// Format setting line
				settingLine := fmt.Sprintf("%s = %s", setting.Key, value)
				if currentIndex == m.cursor {
					settingLine = styles.Selected.Render(settingLine)
				}

				s += fmt.Sprintf("%s%s\n", cursor, settingLine)

				// Show description for selected item
				if currentIndex == m.cursor {
					desc := setting.Description
					if len(setting.PossibleValues) > 0 {
						desc = fmt.Sprintf("%s (%s)", desc, strings.Join(setting.PossibleValues, ", "))
					}
					s += fmt.Sprintf("      %s\n", styles.Help.Render(desc))
				}

				currentIndex++
			}
		}
		s += "\n"
	}

	// Status messages at bottom
	if m.errorMsg != "" {
		s += "\n" + styles.Error.Render("✗ " + m.errorMsg) + "\n"
	} else if m.successMsg != "" {
		s += "\n" + styles.Success.Render(m.successMsg) + "\n"
	}

	// Help text based on mode
	var help string
	if m.mode == modeEditText {
		help = "Enter: save • Esc: cancel • Backspace: delete"
	} else if m.mode == modeEditSelect {
		help = "↑/↓: select • Enter: save • Esc: cancel"
	} else if m.mode == modeEditLocation {
		help = "Tab/Shift+Tab: move field • Enter: save • Esc: cancel"
	} else {
		help = "↑/↓: navigate • Enter: edit • d: delete • Esc: back to menu • q: quit"
	}
	s += "\n" + styles.Help.Render(help)

	settingsList := lipgloss.NewStyle().Margin(2, 4).Render(s)

	// If in edit mode, show dialog centered on screen
	if m.mode == modeEditText || m.mode == modeEditSelect || m.mode == modeEditLocation {
		var dialog string

		if m.mode == modeEditText {
			setting, _, isLocation := m.getCurrentItem()
			if !isLocation && setting != nil {
				dialog = m.renderTextEditDialog(setting)
			}
		} else if m.mode == modeEditSelect {
			setting, _, isLocation := m.getCurrentItem()
			if !isLocation && setting != nil {
				dialog = m.renderSelectDialog(setting)
			}
		} else if m.mode == modeEditLocation {
			dialog = m.renderLocationEditDialog()
		}

		if dialog != "" {
			// Show dialog with margins to position it, no screen clearing
			dialogBox := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2).
				MarginTop(5).
				MarginLeft(30).
				Render(dialog)

			return dialogBox
		}
	}

	return settingsList
}

func (m Settings) renderTextEditDialog(setting *registry.Setting) string {
	var s string

	s += styles.Selected.Render(fmt.Sprintf("Edit: %s", setting.Key)) + "\n\n"

	// Show description
	desc := setting.Description
	if setting.Unit != "" {
		desc = fmt.Sprintf("%s [%s]", desc, setting.Unit)
	}
	s += styles.Help.Render(desc) + "\n\n"

	// Show input
	s += "Value: " + styles.Selected.Render(m.editBuffer+"│") + "\n"

	return s
}

func (m Settings) renderSelectDialog(setting *registry.Setting) string {
	var s string

	s += styles.Selected.Render(fmt.Sprintf("Edit: %s", setting.Key)) + "\n\n"

	// Show description prominently
	desc := setting.Description
	if setting.Unit != "" {
		desc = fmt.Sprintf("%s [%s]", desc, setting.Unit)
	}
	s += lipgloss.NewStyle().
		Foreground(lipgloss.Color("246")).
		Italic(true).
		Render(desc) + "\n\n"

	// Show options with better formatting
	for i, value := range setting.PossibleValues {
		cursor := "  "
		if i == m.selectCursor {
			cursor = "▸ "
		}

		displayValue := value
		if i == m.selectCursor {
			displayValue = styles.Selected.Render(value)
		}

		// Mark current value
		marker := ""
		if value == m.values[setting.Key] {
			marker = " " + styles.Success.Render("✓")
		}

		s += fmt.Sprintf("%s%-20s%s\n", cursor, displayValue, marker)
	}

	// Show default value if set
	if setting.DefaultValue != "" {
		s += "\n" + styles.Help.Render(fmt.Sprintf("Default: %s", setting.DefaultValue))
	}

	return s
}

func (m Settings) renderLocationEditDialog() string {
	var s string

	s += styles.Selected.Render(fmt.Sprintf("Edit Location %d", m.editLocationIdx)) + "\n\n"
	s += styles.Help.Render("Tab/Shift+Tab to move between fields") + "\n\n"

	// Label field
	labelPrefix := "  "
	labelValue := m.editLocationLabel
	if m.editLocationField == fieldLabel {
		labelPrefix = "▸ "
		labelValue = styles.Selected.Render(m.editLocationLabel + "│")
	}
	s += fmt.Sprintf("%sLabel:     %s\n", labelPrefix, labelValue)

	// Latitude field
	latPrefix := "  "
	latValue := m.editLocationLat
	if m.editLocationField == fieldLatitude {
		latPrefix = "▸ "
		latValue = styles.Selected.Render(m.editLocationLat + "│")
	}
	s += fmt.Sprintf("%sLatitude:  %s\n", latPrefix, latValue)

	// Longitude field
	lonPrefix := "  "
	lonValue := m.editLocationLon
	if m.editLocationField == fieldLongitude {
		lonPrefix = "▸ "
		lonValue = styles.Selected.Render(m.editLocationLon + "│")
	}
	s += fmt.Sprintf("%sLongitude: %s\n", lonPrefix, lonValue)

	return s
}
