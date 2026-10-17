package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
)

// Update handles incoming messages and user input
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case EventMsg:
		ev := hub.Event(msg)
		m.events = append(m.events, ev)

		// Check if matches filter
		if m.filterText != "" || m.selectedCamFilter != "" {
			m.ApplyFilter()
		} else {
			m.filteredEvents = m.events
		}

		if !m.paused {
			// Auto scroll to latest event
			if len(m.filteredEvents) > 0 {
				m.eventIndex = len(m.filteredEvents) - 1
			}
		}
		cmds = append(cmds, waitForEvent(m.eventCh))

	case DeviceMsg:
		dev := hub.DeviceStatus(msg)
		found := false
		for i, d := range m.devices {
			if isSameDevice(d, dev) {
				m.devices[i] = dev
				found = true
				break
			}
		}
		if !found {
			m.devices = append(m.devices, dev)
		}
		cmds = append(cmds, waitForDevice(m.deviceCh))

	case tea.KeyMsg:
		if m.isConfiguring {
			switch msg.String() {
			case "esc":
				m.isConfiguring = false
				return m, nil

			case "tab", "down":
				m.configFocus = (m.configFocus + 1) % 5
				for i := range m.configInputs {
					if i == m.configFocus {
						m.configInputs[i].Focus()
					} else {
						m.configInputs[i].Blur()
					}
				}
				return m, nil

			case "shift+tab", "up":
				m.configFocus = (m.configFocus + 4) % 5
				for i := range m.configInputs {
					if i == m.configFocus {
						m.configInputs[i].Focus()
					} else {
						m.configInputs[i].Blur()
					}
				}
				return m, nil

			case "enter":
				name := strings.TrimSpace(m.configInputs[0].Value())
				target := strings.TrimSpace(m.configInputs[1].Value())
				user := strings.TrimSpace(m.configInputs[2].Value())
				pass := m.configInputs[3].Value()

				if user == "" {
					user = "admin"
				}

				if m.appConfig != nil {
					var err error
					if target == "" {
						err = m.appConfig.SetDefaultCredentials(user, pass)
					} else {
						err = m.appConfig.SetCameraCredentials(config.CameraConfig{
							Name:     name,
							Target:   target,
							Username: user,
							Password: pass,
						})
					}
					if err != nil {
						m.configMessage = "Error: " + err.Error()
						m.configErr = true
					} else {
						display := target
						if name != "" {
							display = fmt.Sprintf("%s (%s)", name, target)
						}
						if target == "" {
							m.configMessage = fmt.Sprintf("✓ Default credentials saved to %s", m.appConfig.FilePath())
						} else {
							m.configMessage = fmt.Sprintf("✓ Camera %s saved to %s", display, m.appConfig.FilePath())
						}
						m.configErr = false
						if m.onConfigUpdate != nil {
							m.onConfigUpdate(target, user, pass)
						}
					}
				} else {
					m.configMessage = "Error: Configuration file not initialized"
					m.configErr = true
				}
				return m, nil

			default:
				if m.configFocus >= 0 && m.configFocus < 4 {
					var cmd tea.Cmd
					m.configInputs[m.configFocus], cmd = m.configInputs[m.configFocus].Update(msg)
					return m, cmd
				}
				return m, nil
			}
		}

		if m.isFiltering {
			switch msg.String() {
			case "enter":
				m.filterText = m.filterInput.Value()
				m.isFiltering = false
				m.filterInput.Blur()
				m.ApplyFilter()
				return m, nil
			case "esc":
				m.isFiltering = false
				m.filterInput.SetValue(m.filterText)
				m.filterInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.unsubEvent != nil {
				m.unsubEvent()
			}
			if m.unsubDevice != nil {
				m.unsubDevice()
			}
			return m, tea.Quit

		case "tab":
			m.activePane = (m.activePane + 1) % 3
			return m, nil

		case "shift+tab":
			if m.activePane == 0 {
				m.activePane = PaneInspector
			} else {
				m.activePane--
			}
			return m, nil

		case "c", "e":
			target := ""
			user := "admin"
			name := ""
			if len(m.devices) > 0 && m.deviceIndex >= 0 && m.deviceIndex < len(m.devices) {
				dev := m.devices[m.deviceIndex]
				target = fmt.Sprintf("%s:%d", dev.IP, dev.Port)
				name = dev.Name
			}
			m.OpenConfig(target, user, name)
			return m, textinput.Blink

		case "a":
			m.OpenConfig("", "admin", "")
			return m, textinput.Blink

		case "d":
			du := "admin"
			if m.appConfig != nil {
				if u, _ := m.appConfig.GetDefaultCredentials(); u != "" {
					du = u
				}
			}
			m.OpenConfig("", du, "")
			m.configFocus = 2
			for i := range m.configInputs {
				if i == 2 {
					m.configInputs[i].Focus()
				} else {
					m.configInputs[i].Blur()
				}
			}
			return m, textinput.Blink

		case "esc":
			if m.selectedCamFilter != "" || m.filterText != "" {
				m.selectedCamFilter = ""
				m.selectedCamName = ""
				m.filterText = ""
				m.filterInput.SetValue("")
				m.ApplyFilter()
				return m, nil
			}
			return m, nil

		case "backspace", "A":
			if m.selectedCamFilter != "" {
				m.selectedCamFilter = ""
				m.selectedCamName = ""
				m.ApplyFilter()
				return m, nil
			}
			return m, nil

		case "enter":
			if m.activePane == PaneDevices && len(m.devices) > 0 && m.deviceIndex >= 0 && m.deviceIndex < len(m.devices) {
				dev := m.devices[m.deviceIndex]
				if m.isDeviceFiltered(dev) {
					// Toggle off: clear camera filter to show all cameras
					m.selectedCamFilter = ""
					m.selectedCamName = ""
				} else {
					// Set camera filter for this camera
					if dev.Name != "" {
						m.selectedCamName = dev.Name
					} else {
						m.selectedCamName = fmt.Sprintf("%s:%d", dev.IP, dev.Port)
					}
					if dev.IP != "" {
						m.selectedCamFilter = dev.IP
					} else if dev.Endpoint != "" {
						m.selectedCamFilter = dev.Endpoint
					} else {
						m.selectedCamFilter = dev.Name
					}
				}
				m.ApplyFilter()
				return m, nil
			}
			return m, nil

		case "/":
			m.isFiltering = true
			m.filterInput.Focus()
			return m, m.filterInput.Focus()

		case " ":
			m.paused = !m.paused
			return m, nil

		case "x":
			m.showRawXML = !m.showRawXML
			m.inspectorScroll = 0
			return m, nil

		case "up", "k":
			switch m.activePane {
			case PaneDevices:
				if m.deviceIndex > 0 {
					m.deviceIndex--
				}
			case PaneFeed:
				if m.eventIndex > 0 {
					m.eventIndex--
				}
			case PaneInspector:
				if m.inspectorScroll > 0 {
					m.inspectorScroll--
				}
			}

		case "down", "j":
			switch m.activePane {
			case PaneDevices:
				if m.deviceIndex < len(m.devices)-1 {
					m.deviceIndex++
				}
			case PaneFeed:
				if m.eventIndex < len(m.filteredEvents)-1 {
					m.eventIndex++
				}
			case PaneInspector:
				m.inspectorScroll++

			}

		case "g", "home":
			switch m.activePane {
			case PaneDevices:
				m.deviceIndex = 0
			case PaneFeed:
				m.eventIndex = 0
			case PaneInspector:
				m.inspectorScroll = 0
			}

		case "G", "end":
			switch m.activePane {
			case PaneDevices:
				if len(m.devices) > 0 {
					m.deviceIndex = len(m.devices) - 1
				}
			case PaneFeed:
				if len(m.filteredEvents) > 0 {
					m.eventIndex = len(m.filteredEvents) - 1
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func isSameDevice(a, b hub.DeviceStatus) bool {
	if a.Endpoint != "" && b.Endpoint != "" {
		return a.Endpoint == b.Endpoint
	}
	if a.IP == b.IP && a.Port == b.Port && a.Port != 0 {
		return true
	}
	if a.Endpoint == "" && b.Endpoint == "" && a.IP == b.IP && a.Port == b.Port {
		return true
	}
	return false
}
