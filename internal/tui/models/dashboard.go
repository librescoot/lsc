package models

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/tui/styles"
)

type Dashboard struct {
	redisClient *redis.Client
	vehicle     map[string]string
	engineECU   map[string]string
	battery0    map[string]string
	battery1    map[string]string
	gps         map[string]string
	alarm       map[string]string
	modem       map[string]string
	powerMux    map[string]string
	width       int
	height      int
	lastUpdate  time.Time
	quitting    bool
}

func NewDashboard(redisClient *redis.Client) Dashboard {
	return Dashboard{
		redisClient: redisClient,
		vehicle:     make(map[string]string),
		engineECU:   make(map[string]string),
		battery0:    make(map[string]string),
		battery1:    make(map[string]string),
		gps:         make(map[string]string),
		alarm:       make(map[string]string),
		modem:       make(map[string]string),
		powerMux:    make(map[string]string),
		lastUpdate:  time.Now(),
	}
}

func (m Dashboard) Init() tea.Cmd {
	return tea.Batch(
		tea.ClearScreen,
		m.fetchData,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tickMsg time.Time

type dataLoadedMsg struct {
	vehicle   map[string]string
	engineECU map[string]string
	battery0  map[string]string
	battery1  map[string]string
	gps       map[string]string
	alarm     map[string]string
	modem     map[string]string
	powerMux  map[string]string
}

func (m Dashboard) fetchData() tea.Msg {
	vehicle, _ := m.redisClient.HGetAll("vehicle")
	engineECU, _ := m.redisClient.HGetAll("engine-ecu")
	battery0, _ := m.redisClient.HGetAll("battery:0")
	battery1, _ := m.redisClient.HGetAll("battery:1")
	gps, _ := m.redisClient.HGetAll("gps")
	alarm, _ := m.redisClient.HGetAll("alarm")
	modem, _ := m.redisClient.HGetAll("modem")
	powerMux, _ := m.redisClient.HGetAll("power-mux")

	return dataLoadedMsg{
		vehicle:   vehicle,
		engineECU: engineECU,
		battery0:  battery0,
		battery1:  battery1,
		gps:       gps,
		alarm:     alarm,
		modem:     modem,
		powerMux:  powerMux,
	}
}

func (m Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case dataLoadedMsg:
		m.vehicle = msg.vehicle
		m.engineECU = msg.engineECU
		m.battery0 = msg.battery0
		m.battery1 = msg.battery1
		m.gps = msg.gps
		m.alarm = msg.alarm
		m.modem = msg.modem
		m.powerMux = msg.powerMux
		m.lastUpdate = time.Now()

	case tickMsg:
		// Auto-refresh every second
		return m, tea.Batch(m.fetchData, tickCmd())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			// Return to main menu
			menu := NewMenu("", m.redisClient)
			return menu, menu.Init()

		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Dashboard) View() string {
	if m.quitting {
		return ""
	}

	s := styles.Title.Render("Live Dashboard - E-Moped Monitor")
	s += "\n\n"

	// Two-column layout: left side for main info, right side for status
	leftCol := m.renderLeftColumn()
	rightCol := m.renderRightColumn()

	// Combine columns if we have width info
	if m.width > 80 {
		leftWidth := int(float64(m.width) * 0.6)
		rightWidth := m.width - leftWidth
		leftPanel := lipgloss.NewStyle().Width(leftWidth).Render(leftCol)
		rightPanel := lipgloss.NewStyle().Width(rightWidth).Render(rightCol)
		s += lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	} else {
		s += leftCol + "\n" + rightCol
	}

	// Help text at bottom
	help := fmt.Sprintf("Auto-refresh (last: %s) • Esc: menu • q: quit",
		m.lastUpdate.Format("15:04:05"))
	s += "\n" + styles.Help.Render(help)

	return lipgloss.NewStyle().Margin(2, 4).Render(s)
}

func (m Dashboard) renderLeftColumn() string {
	var s string

	// Vehicle State
	s += styles.Subtitle.Render("▸ Vehicle") + "\n"
	s += fmt.Sprintf("  State: %s", format.ColorizeState(m.vehicle["state"]))
	if ts := m.vehicle["state:timestamp"]; ts != "" {
		s += styles.Help.Render(fmt.Sprintf(" (%s)", ts))
	}
	s += "\n"
	s += fmt.Sprintf("  Kickstand: %s\n", m.vehicle["kickstand"])
	s += fmt.Sprintf("  Brakes: L:%s R:%s\n", m.vehicle["brake:left"], m.vehicle["brake:right"])
	s += fmt.Sprintf("  Handlebar: %s (lock: %s)\n",
		m.vehicle["handlebar:position"], m.vehicle["handlebar:lock-sensor"])
	s += fmt.Sprintf("  Seatbox: %s\n", m.vehicle["seatbox:lock"])
	s += fmt.Sprintf("  Blinkers: %s\n", m.vehicle["blinker:state"])
	if dbcUpdate := m.vehicle["dbc-updating"]; dbcUpdate == "true" {
		s += "  " + styles.Error.Render("⚠ DBC Updating") + "\n"
	}
	s += "\n"

	// Engine/Motor
	s += styles.Subtitle.Render("▸ Motor & Drive") + "\n"
	if speed := m.engineECU["speed"]; speed != "" {
		s += fmt.Sprintf("  Speed: %s\n", format.FormatSpeed(speed))
	}
	if odo := m.engineECU["odometer"]; odo != "" {
		s += fmt.Sprintf("  Odometer: %s m\n", odo)
	}
	if temp := m.engineECU["temperature"]; temp != "" {
		s += fmt.Sprintf("  Temperature: %s\n", format.FormatTemperatureColored(temp))
	}
	if voltage := m.engineECU["motor:voltage"]; voltage != "" {
		s += fmt.Sprintf("  Motor Voltage: %s\n", format.FormatVoltageColored(voltage))
	}
	if current := m.engineECU["motor:current"]; current != "" {
		s += fmt.Sprintf("  Motor Current: %s\n", format.FormatAmperageColored(current))
	}
	s += "\n"

	// Batteries
	s += styles.Subtitle.Render("▸ Battery System") + "\n"
	s += m.renderBattery("0", m.battery0)
	if m.battery1["present"] == "true" {
		s += m.renderBattery("1", m.battery1)
	}
	if powerSrc := m.powerMux["selected-input"]; powerSrc != "" {
		s += fmt.Sprintf("  Power Source: %s\n", powerSrc)
	}

	return s
}

func (m Dashboard) renderBattery(id string, bat map[string]string) string {
	var s string
	state := bat["state"]
	if state == "" {
		return ""
	}

	s += fmt.Sprintf("  Battery %s [%s]:\n", id, state)
	if soc := bat["charge"]; soc != "" {
		s += fmt.Sprintf("    SoC: %s ", format.FormatPercentage(soc))
		if soh := bat["state-of-health"]; soh != "" {
			s += styles.Help.Render(fmt.Sprintf("(SoH: %s%%)", soh))
		}
		s += "\n"
	}
	if voltage := bat["voltage"]; voltage != "" {
		s += fmt.Sprintf("    Voltage: %s\n", format.FormatVoltageColored(voltage))
	}
	if current := bat["current"]; current != "" {
		s += fmt.Sprintf("    Current: %s\n", format.FormatAmperageColored(current))
	}
	if tempState := bat["temperature-state"]; tempState != "" {
		s += fmt.Sprintf("    Temp: %s", tempState)
		if temp := bat["temperature"]; temp != "" {
			s += fmt.Sprintf(" (%s)", format.FormatTemperatureColored(temp))
		}
		s += "\n"
	}
	if cycles := bat["cycle-count"]; cycles != "" {
		s += styles.Help.Render(fmt.Sprintf("    Cycles: %s", cycles))
		if serial := bat["serial-number"]; serial != "" {
			s += styles.Help.Render(fmt.Sprintf(" • SN: %s", serial[len(serial)-8:]))
		}
		s += "\n"
	}

	return s
}

func (m Dashboard) renderRightColumn() string {
	var s string

	// GPS
	s += styles.Subtitle.Render("▸ GPS") + "\n"
	gpsState := m.gps["state"]
	if gpsState == "no-fix" || gpsState == "" {
		s += "  " + styles.Error.Render("No Fix") + "\n"
	} else {
		s += fmt.Sprintf("  Status: %s\n", gpsState)
	}
	if quality := m.gps["quality"]; quality != "" {
		s += fmt.Sprintf("  Quality: %s\n", quality)
	}
	if updated := m.gps["updated"]; updated != "" {
		s += styles.Help.Render(fmt.Sprintf("  Updated: %s", updated)) + "\n"
	}
	s += "\n"

	// Alarm
	s += styles.Subtitle.Render("▸ Alarm") + "\n"
	alarmStatus := m.alarm["status"]
	if alarmStatus == "armed" {
		s += "  " + styles.Success.Render("Armed ✓") + "\n"
	} else if alarmStatus == "disarmed" {
		s += "  " + styles.Help.Render("Disarmed") + "\n"
	} else {
		s += fmt.Sprintf("  Status: %s\n", alarmStatus)
	}
	s += "\n"

	// Connectivity
	s += styles.Subtitle.Render("▸ Connectivity") + "\n"
	if operator := m.modem["operator-name"]; operator != "" {
		s += fmt.Sprintf("  Network: %s\n", operator)
	}
	if simState := m.modem["sim-state"]; simState != "" {
		if simState == "locked" {
			s += "  SIM: " + styles.Error.Render("Locked") + "\n"
		} else {
			s += fmt.Sprintf("  SIM: %s\n", simState)
		}
	}
	if errorState := m.modem["error-state"]; errorState != "" && errorState != "none" {
		s += "  " + styles.Error.Render(fmt.Sprintf("Error: %s", errorState)) + "\n"
	}

	return s
}
