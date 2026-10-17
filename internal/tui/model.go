package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
)

// ActivePane represents the currently focused pane
type ActivePane int

const (
	PaneDevices ActivePane = iota
	PaneFeed
	PaneInspector
)

// Messages for Bubbletea event loop
type EventMsg hub.Event
type DeviceMsg hub.DeviceStatus

// ConfigUpdateFunc is called when camera settings are updated in the TUI
type ConfigUpdateFunc func(target, username, password string)

// Options holds optional dependencies for the TUI Model
type Options struct {
	AppConfig      *config.Config
	OnConfigUpdate ConfigUpdateFunc
}

// Model is the main state of the Terminal User Interface
type Model struct {
	hub            *hub.Hub
	eventCh        <-chan hub.Event
	deviceCh       <-chan hub.DeviceStatus
	unsubEvent     func()
	unsubDevice    func()

	appConfig      *config.Config
	onConfigUpdate ConfigUpdateFunc

	activePane     ActivePane
	devices        []hub.DeviceStatus
	deviceIndex    int

	events         []hub.Event
	filteredEvents []hub.Event
	eventIndex     int

	paused         bool
	showRawXML     bool
	inspectorScroll int

	filterInput       textinput.Model
	isFiltering       bool
	filterText        string
	selectedCamFilter string // Camera ID, Endpoint, or IP:Port currently filtering the live feed
	selectedCamName   string // Display name of selected camera

	// Camera Configuration Dialog State
	isConfiguring  bool
	configInputs   [4]textinput.Model // 0: Name, 1: Target, 2: Username, 3: Password
	configFocus    int                // 0, 1, 2, 3, or 4 (Save button)
	configMessage  string             // Status / error message
	configErr      bool

	width          int
	height         int
	ready          bool
}

// NewModel initializes the TUI model
func NewModel(h *hub.Hub, opts ...Options) Model {
	ti := textinput.New()
	ti.Placeholder = "Filter by topic or IP (press Enter to apply, Esc to cancel)..."
	ti.CharLimit = 100
	ti.Width = 40

	var cfg *config.Config
	var onUpdate ConfigUpdateFunc
	if len(opts) > 0 {
		cfg = opts[0].AppConfig
		onUpdate = opts[0].OnConfigUpdate
	}

	var cfgInputs [4]textinput.Model
	cfgInputs[0] = textinput.New()
	cfgInputs[0].Placeholder = "e.g. Garden, camera1, Front Door (Optional)"
	cfgInputs[0].CharLimit = 64
	cfgInputs[0].Width = 36

	cfgInputs[1] = textinput.New()
	cfgInputs[1].Placeholder = "192.168.1.100:80 (or leave empty for default)"
	cfgInputs[1].CharLimit = 64
	cfgInputs[1].Width = 36

	cfgInputs[2] = textinput.New()
	cfgInputs[2].Placeholder = "admin"
	cfgInputs[2].CharLimit = 64
	cfgInputs[2].Width = 36

	cfgInputs[3] = textinput.New()
	cfgInputs[3].Placeholder = "••••••••"
	cfgInputs[3].EchoMode = textinput.EchoPassword
	cfgInputs[3].EchoCharacter = '•'
	cfgInputs[3].CharLimit = 64
	cfgInputs[3].Width = 36

	eventCh, unsubEv := h.SubscribeEvents(200)
	devCh, unsubDev := h.SubscribeDevices(50)

	history := h.GetHistory(1000)
	devices := h.GetDevices()

	m := Model{
		hub:            h,
		eventCh:        eventCh,
		deviceCh:       devCh,
		unsubEvent:     unsubEv,
		unsubDevice:    unsubDev,
		appConfig:      cfg,
		onConfigUpdate: onUpdate,
		activePane:     PaneFeed,
		devices:        devices,
		events:         history,
		filteredEvents: history,
		filterInput:    ti,
		configInputs:   cfgInputs,
		eventIndex:     0,
	}

	if len(history) > 0 {
		m.eventIndex = len(history) - 1
	}

	return m
}

// OpenConfig opens the camera configuration dialog prefilling name, target, and credentials
func (m *Model) OpenConfig(target, username, name string) {
	m.isConfiguring = true
	m.configMessage = ""
	m.configErr = false

	if target != "" {
		if name == "" && m.appConfig != nil {
			name = m.appConfig.GetCameraName(target, 0, "", "")
		}
		m.configInputs[0].SetValue(name)
		m.configInputs[1].SetValue(target)
		if username != "" && username != "admin" {
			m.configInputs[2].SetValue(username)
		} else if m.appConfig != nil {
			u, _, matched := m.appConfig.GetCredentials(target, 0, "", "")
			if matched && u != "" {
				m.configInputs[2].SetValue(u)
			} else {
				du, _ := m.appConfig.GetDefaultCredentials()
				if du != "" {
					m.configInputs[2].SetValue(du)
				} else {
					m.configInputs[2].SetValue("admin")
				}
			}
		} else {
			m.configInputs[2].SetValue("admin")
		}
		m.configInputs[3].SetValue("")
		m.configFocus = 3 // focus password directly for convenience
	} else {
		m.configInputs[0].SetValue(name)
		m.configInputs[1].SetValue("")
		if m.appConfig != nil {
			du, _ := m.appConfig.GetDefaultCredentials()
			if du != "" {
				m.configInputs[2].SetValue(du)
			} else {
				m.configInputs[2].SetValue("admin")
			}
		} else {
			m.configInputs[2].SetValue("admin")
		}
		m.configInputs[3].SetValue("")
		m.configFocus = 0 // focus Name input first
	}

	for i := range m.configInputs {
		if i == m.configFocus {
			m.configInputs[i].Focus()
		} else {
			m.configInputs[i].Blur()
		}
	}
}

// Init initializes the subscription listening commands
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.eventCh),
		waitForDevice(m.deviceCh),
	)
}

func waitForEvent(ch <-chan hub.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return EventMsg(ev)
	}
}

func waitForDevice(ch <-chan hub.DeviceStatus) tea.Cmd {
	return func() tea.Msg {
		dev, ok := <-ch
		if !ok {
			return nil
		}
		return DeviceMsg(dev)
	}
}

// isDeviceFiltered returns true if dev matches the currently active camera filter
func (m *Model) isDeviceFiltered(dev hub.DeviceStatus) bool {
	if m.selectedCamFilter == "" {
		return false
	}
	addr := fmt.Sprintf("%s:%d", dev.IP, dev.Port)
	if m.selectedCamFilter == dev.IP || m.selectedCamFilter == addr {
		return true
	}
	if dev.Endpoint != "" && (m.selectedCamFilter == dev.Endpoint || strings.HasPrefix(m.selectedCamFilter, dev.Endpoint)) {
		return true
	}
	if dev.Name != "" && (strings.EqualFold(m.selectedCamFilter, dev.Name) || (m.selectedCamName != "" && strings.EqualFold(m.selectedCamName, dev.Name))) {
		return true
	}
	return false
}

// ApplyFilter re-evaluates filteredEvents based on current filterText and selectedCamFilter
func (m *Model) ApplyFilter() {
	q := strings.ToLower(m.filterText)
	var filtered []hub.Event

	for _, e := range m.events {
		// 1. Camera filter
		if m.selectedCamFilter != "" {
			matches := e.CameraID == m.selectedCamFilter ||
				e.CameraIP == m.selectedCamFilter ||
				(e.CameraName != "" && strings.EqualFold(e.CameraName, m.selectedCamFilter)) ||
				strings.HasPrefix(m.selectedCamFilter, e.CameraIP+":") ||
				(e.CameraIP != "" && strings.HasPrefix(e.CameraIP+":", m.selectedCamFilter)) ||
				strings.HasPrefix(e.CameraID, m.selectedCamFilter) ||
				(m.selectedCamName != "" && e.CameraName != "" && strings.EqualFold(e.CameraName, m.selectedCamName))
			if !matches {
				continue
			}
		}

		// 2. Search query filter
		if q != "" {
			topic := strings.ToLower(e.Topic)
			ip := strings.ToLower(e.CameraIP)
			camName := strings.ToLower(e.CameraName)
			cat := strings.ToLower(e.Category)
			prop := strings.ToLower(e.PropertyState)

			if !strings.Contains(topic, q) &&
				!strings.Contains(ip, q) &&
				!strings.Contains(camName, q) &&
				!strings.Contains(cat, q) &&
				!strings.Contains(prop, q) {
				continue
			}
		}

		filtered = append(filtered, e)
	}

	m.filteredEvents = filtered
	if m.eventIndex >= len(m.filteredEvents) {
		m.eventIndex = max(0, len(m.filteredEvents)-1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
