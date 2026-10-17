package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the 3-pane interface
func (m Model) View() string {
	if !m.ready || m.width < 40 || m.height < 10 {
		return "Initializing ONVIF-INSPECTOR..."
	}

	if m.isConfiguring {
		return m.renderConfigModal()
	}

	headerView := m.renderHeader()
	helpView := m.renderHelp()

	headerHeight := lipgloss.Height(headerView)
	helpHeight := lipgloss.Height(helpView)

	targetMainHeight := m.height - headerHeight - helpHeight
	if targetMainHeight < 8 {
		targetMainHeight = 8
	}

	// Width distribution: exactly sums to m.width
	leftTotalWidth := m.width * 33 / 100
	if leftTotalWidth < 28 {
		leftTotalWidth = 28
	}
	if leftTotalWidth > m.width-36 {
		leftTotalWidth = m.width - 36
	}
	rightTotalWidth := m.width - leftTotalWidth

	// Lipgloss frame sizes for StyleBorderNormal and StyleBorderActive
	// (RoundedBorder = 2 horizontal, 2 vertical; Padding(0, 1) = 2 horizontal, 0 vertical)
	const vFrame = 2

	// The right column contains feedPane (top) and inspectorPane (bottom).
	// Total vertical border lines in right column = 2 * vFrame = 4.
	availRightContentHeight := targetMainHeight - (2 * vFrame)
	if availRightContentHeight < 2 {
		availRightContentHeight = 2
	}
	feedContentHeight := availRightContentHeight / 2
	inspectorContentHeight := availRightContentHeight - feedContentHeight

	leftPane := m.renderDevicesPane(leftTotalWidth, targetMainHeight)
	feedPane := m.renderFeedPane(rightTotalWidth, feedContentHeight)
	inspectorPane := m.renderInspectorPane(rightTotalWidth, inspectorContentHeight)

	rightPanes := lipgloss.JoinVertical(lipgloss.Left, feedPane, inspectorPane)
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPanes)

	return lipgloss.JoinVertical(lipgloss.Left, headerView, mainContent, helpView)
}

func (m Model) renderHeader() string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#4F46E5")).
		Padding(0, 1).
		Render("ONVIF-INSPECTOR")

	stats := fmt.Sprintf("  📹 Cameras: %d  |  ⚡ Events: %d", len(m.devices), len(m.events))
	statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Bold(true).Render(stats)

	var badges []string
	if m.paused {
		badges = append(badges, StylePausedBadge.Render("⏸ PAUSED"))
	}
	if m.selectedCamFilter != "" {
		filterLabel := m.selectedCamName
		if filterLabel == "" {
			filterLabel = m.selectedCamFilter
		}
		badges = append(badges, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2563EB")).
			Padding(0, 1).
			Bold(true).
			Render("📹 Filter: "+filterLabel+" (Esc: All)"))
	}
	if m.filterText != "" {
		badges = append(badges, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSecondary).
			Padding(0, 1).
			Bold(true).
			Render("🔍 Filter: "+m.filterText))
	}

	badgesStr := strings.Join(badges, " ")
	left := lipgloss.JoinHorizontal(lipgloss.Center, logo, statsStyle)
	right := badgesStr

	space := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if space < 1 {
		space = 1
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Center, left, strings.Repeat(" ", space), right)
	return StyleHeader.Width(m.width - 2).Render(bar)
}

func (m Model) renderDevicesPane(totalWidth, totalHeight int) string {
	style := StyleBorderNormal
	titleStyle := StylePaneTitle
	if m.activePane == PaneDevices {
		style = StyleBorderActive
		titleStyle = StylePaneTitleActive
	}

	contentWidth := totalWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}
	contentHeight := totalHeight - 2
	if contentHeight < 2 {
		contentHeight = 2
	}

	title := titleStyle.Render(fmt.Sprintf(" DEVICES (%d) ", len(m.devices)))

	var rows []string
	if len(m.devices) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorMuted).Render("Discovering cameras..."))
	} else {
		// Available vertical lines for list items
		availLines := contentHeight - 1
		if availLines < 2 {
			availLines = 2
		}
		visibleCount := availLines / 2
		if visibleCount < 1 {
			visibleCount = 1
		}

		if m.deviceIndex >= len(m.devices) {
			m.deviceIndex = len(m.devices) - 1
		}
		if m.deviceIndex < 0 {
			m.deviceIndex = 0
		}

		// Calculate scrolling window around m.deviceIndex
		startIdx := 0
		if len(m.devices) > visibleCount {
			startIdx = m.deviceIndex - visibleCount/2
			if startIdx < 0 {
				startIdx = 0
			}
			if startIdx+visibleCount > len(m.devices) {
				startIdx = len(m.devices) - visibleCount
			}
		}
		endIdx := startIdx + visibleCount
		if endIdx > len(m.devices) {
			endIdx = len(m.devices)
		}

		if startIdx > 0 && endIdx < len(m.devices) {
			title = titleStyle.Render(fmt.Sprintf(" DEVICES (%d) ⇅ ", len(m.devices)))
		} else if startIdx > 0 {
			title = titleStyle.Render(fmt.Sprintf(" DEVICES (%d) ▲ ", len(m.devices)))
		} else if endIdx < len(m.devices) {
			title = titleStyle.Render(fmt.Sprintf(" DEVICES (%d) ▼ ", len(m.devices)))
		}

		for i := startIdx; i < endIdx; i++ {
			dev := m.devices[i]
			isFiltered := m.isDeviceFiltered(dev)
			isSelected := (i == m.deviceIndex && m.activePane == PaneDevices)

			badge := RenderStateBadge(dev.State)
			addr := fmt.Sprintf("%s:%d", dev.IP, dev.Port)

			// Camera Name display:
			// Priority: dev.Name > dev.Model > dev.IP
			camName := dev.Name
			if camName == "" {
				if dev.Model != "" {
					camName = dev.Model
				} else {
					camName = addr
				}
			}

			prefix := "  "
			if i == m.deviceIndex {
				prefix = "▸ "
			}

			filterTag := ""
			if isFiltered {
				filterTag = " [FILTER]"
			}

			badgeWidth := lipgloss.Width(badge)
			filterWidth := lipgloss.Width(filterTag)
			prefixWidth := lipgloss.Width(prefix)

			fixedWidth := prefixWidth + badgeWidth + filterWidth + 2
			availNameWidth := contentWidth - fixedWidth
			if availNameWidth < 4 {
				if filterWidth > 0 && contentWidth-prefixWidth-badgeWidth-2 >= 4 {
					filterTag = " [F]"
					filterWidth = lipgloss.Width(filterTag)
					availNameWidth = contentWidth - prefixWidth - badgeWidth - filterWidth - 2
				}
				if availNameWidth < 2 {
					availNameWidth = 2
				}
			}
			truncatedName := truncate(camName, availNameWidth)
			nameWidth := lipgloss.Width(truncatedName)
			padding := contentWidth - prefixWidth - nameWidth - badgeWidth - filterWidth
			if padding < 1 {
				padding = 1
			}
			line1 := fmt.Sprintf("%s%s%s%s%s", prefix, truncatedName, strings.Repeat(" ", padding), badge, filterTag)

			// Line 2: target addr | model | event count | latency
			latStr := ""
			if dev.PingLatency > 0 {
				latStr = fmt.Sprintf(" (%v)", dev.PingLatency.Round(100*time.Microsecond))
			}
			var details []string
			if addr != "" {
				details = append(details, addr)
			}
			if dev.Model != "" && dev.Model != camName {
				details = append(details, truncate(dev.Model, 12))
			}
			details = append(details, fmt.Sprintf("Ev:%d%s", dev.EventCount, latStr))
			line2Text := strings.Join(details, " | ")
			line2 := "    " + truncate(line2Text, contentWidth-4)

			rowBlock := fmt.Sprintf("%s\n%s", line1, line2)
			if isSelected {
				rowBlock = StyleDeviceRowSelected.Width(contentWidth).Render(rowBlock)
			} else if isFiltered {
				rowBlock = StyleRowFiltered.Width(contentWidth).Render(rowBlock)
			} else {
				rowBlock = StyleRowNormal.Width(contentWidth).Render(rowBlock)
			}

			rows = append(rows, rowBlock)
		}
	}

	body := strings.Join(rows, "\n")
	paneContent := fmt.Sprintf("%s\n%s", title, body)
	return style.Width(totalWidth - 2).Height(contentHeight).Render(paneContent)
}

func (m Model) renderFeedPane(totalWidth, contentHeight int) string {
	style := StyleBorderNormal
	titleStyle := StylePaneTitle
	if m.activePane == PaneFeed {
		style = StyleBorderActive
		titleStyle = StylePaneTitleActive
	}

	contentWidth := totalWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	feedTitle := fmt.Sprintf(" LIVE FEED (%d) ", len(m.filteredEvents))
	if m.selectedCamFilter != "" {
		filterLabel := m.selectedCamName
		if filterLabel == "" {
			filterLabel = m.selectedCamFilter
		}
		feedTitle = fmt.Sprintf(" LIVE FEED [%s] (%d) • Esc:All ", filterLabel, len(m.filteredEvents))
	}
	title := titleStyle.Render(feedTitle)

	var rows []string
	if len(m.filteredEvents) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorMuted).Render("No events received yet..."))
	} else {
		availLines := contentHeight - 1
		if availLines < 1 {
			availLines = 1
		}

		if m.eventIndex >= len(m.filteredEvents) {
			m.eventIndex = len(m.filteredEvents) - 1
		}
		if m.eventIndex < 0 {
			m.eventIndex = 0
		}

		startIdx := 0
		if len(m.filteredEvents) > availLines {
			startIdx = m.eventIndex - availLines/2
			if startIdx < 0 {
				startIdx = 0
			}
			if startIdx+availLines > len(m.filteredEvents) {
				startIdx = len(m.filteredEvents) - availLines
			}
		}

		endIdx := startIdx + availLines
		if endIdx > len(m.filteredEvents) {
			endIdx = len(m.filteredEvents)
		}

		if startIdx > 0 && endIdx < len(m.filteredEvents) {
			title = titleStyle.Render(fmt.Sprintf("%s ⇅ ", strings.TrimSpace(feedTitle)))
		} else if startIdx > 0 {
			title = titleStyle.Render(fmt.Sprintf("%s ▲ ", strings.TrimSpace(feedTitle)))
		} else if endIdx < len(m.filteredEvents) {
			title = titleStyle.Render(fmt.Sprintf("%s ▼ ", strings.TrimSpace(feedTitle)))
		}

		for i := startIdx; i < endIdx; i++ {
			ev := m.filteredEvents[i]
			ts := ev.Timestamp.Format("15:04:05")
			badge := RenderCategoryBadge(ev.Category)

			camName := ev.CameraIP
			if ev.CameraName != "" {
				camName = ev.CameraName
			}
			shortTopic := shortenTopic(ev.Topic)

			prefix := "  "
			if i == m.eventIndex && m.activePane == PaneFeed {
				prefix = "▸ "
			}

			line := fmt.Sprintf("%s%s %s %-12s %-20s %s",
				prefix,
				lipgloss.NewStyle().Foreground(ColorMuted).Render(ts),
				badge,
				truncate(camName, 12),
				truncate(shortTopic, 20),
				lipgloss.NewStyle().Foreground(ColorActive).Bold(true).Render(ev.PropertyState),
			)

			if i == m.eventIndex && m.activePane == PaneFeed {
				line = StyleRowSelected.Width(contentWidth).Render(line)
			} else {
				line = StyleRowNormal.Width(contentWidth).Render(line)
			}

			rows = append(rows, line)
		}
	}

	body := strings.Join(rows, "\n")
	paneContent := fmt.Sprintf("%s\n%s", title, body)
	return style.Width(totalWidth - 2).Height(contentHeight).Render(paneContent)
}

func (m Model) renderInspectorPane(totalWidth, contentHeight int) string {
	style := StyleBorderNormal
	titleStyle := StylePaneTitle
	if m.activePane == PaneInspector {
		style = StyleBorderActive
		titleStyle = StylePaneTitleActive
	}

	contentWidth := totalWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	modeTag := "[Parsed]"
	if m.showRawXML {
		modeTag = "[Raw XML (press 'x' to toggle)]"
	}
	title := titleStyle.Render(fmt.Sprintf(" INSPECTOR %s ", modeTag))

	if len(m.filteredEvents) == 0 || m.eventIndex < 0 || m.eventIndex >= len(m.filteredEvents) {
		body := lipgloss.NewStyle().Foreground(ColorMuted).Render("Select an event in Live Feed to inspect details.")
		return style.Width(totalWidth - 2).Height(contentHeight).Render(fmt.Sprintf("%s\n%s", title, body))
	}

	ev := m.filteredEvents[m.eventIndex]
	availLines := contentHeight - 1
	if availLines < 1 {
		availLines = 1
	}

	var lines []string
	if m.showRawXML {
		raw := ev.RawXML
		if raw == "" {
			raw = "(No raw XML captured for this event)"
		}
		rawLines := strings.Split(raw, "\n")
		start := m.inspectorScroll
		if start >= len(rawLines) {
			start = max(0, len(rawLines)-availLines)
		}
		end := start + availLines
		if end > len(rawLines) {
			end = len(rawLines)
		}
		lines = append(lines, rawLines[start:end]...)
	} else {
		lines = append(lines, fmt.Sprintf("Event ID:    %s", ev.ID))
		lines = append(lines, fmt.Sprintf("Timestamp:   %s", ev.Timestamp.Format("2006-01-02 15:04:05.000 MST")))
		camStr := fmt.Sprintf("%s (%s)", ev.CameraIP, ev.CameraID)
		if ev.CameraName != "" {
			camStr = fmt.Sprintf("%s [%s]", ev.CameraName, ev.CameraIP)
		}
		lines = append(lines, fmt.Sprintf("Camera:      %s", camStr))
		lines = append(lines, fmt.Sprintf("Category:    %s  |  State: %s", ev.Category, ev.PropertyState))
		lines = append(lines, fmt.Sprintf("Topic:       %s", ev.Topic))

		if len(ev.Data) > 0 {
			lines = append(lines, "Properties:")
			for k, v := range ev.Data {
				lines = append(lines, fmt.Sprintf("  • %s = %s", k, v))
			}
		}

		if len(ev.Source) > 0 {
			lines = append(lines, "Source Tokens:")
			for k, v := range ev.Source {
				lines = append(lines, fmt.Sprintf("  • %s = %s", k, v))
			}
		}
	}

	if len(lines) > availLines {
		lines = lines[:availLines]
	}

	body := strings.Join(lines, "\n")
	paneContent := fmt.Sprintf("%s\n%s", title, body)
	return style.Width(totalWidth - 2).Height(contentHeight).Render(paneContent)
}

func (m Model) renderHelp() string {
	if m.isConfiguring {
		return StyleHelp.Width(m.width - 2).Render(" Tab: Next Field | Enter: Save | Esc: Cancel | Saved to config.yaml")
	}
	if m.isFiltering {
		return StyleHelp.Width(m.width - 2).Render(" Filter: " + m.filterInput.View())
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#93C5FD")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	keys := []string{
		keyStyle.Render("Tab") + descStyle.Render(":Pane"),
		keyStyle.Render("↑/↓") + descStyle.Render(":Nav"),
	}

	if m.selectedCamFilter != "" {
		keys = append(keys, keyStyle.Render("Esc/Enter")+descStyle.Render(":All Cams"))
	} else if m.activePane == PaneDevices {
		keys = append(keys, keyStyle.Render("Enter")+descStyle.Render(":Filter Cam"))
	}

	keys = append(keys,
		keyStyle.Render("c")+descStyle.Render(":Config"),
		keyStyle.Render("a")+descStyle.Render(":Add"),
		keyStyle.Render("d")+descStyle.Render(":Defaults"),
		keyStyle.Render("/")+descStyle.Render(":Search"),
		keyStyle.Render("x")+descStyle.Render(":XML"),
		keyStyle.Render("Space")+descStyle.Render(":Pause"),
		keyStyle.Render("q")+descStyle.Render(":Quit"),
	)

	helpBar := strings.Join(keys, "  ")
	return StyleHelp.Width(m.width - 2).Render(helpBar)
}

func (m Model) renderConfigModal() string {
	boxWidth := 58
	if m.width-4 < boxWidth {
		boxWidth = m.width - 4
	}
	if boxWidth < 36 {
		boxWidth = 36
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(ColorPrimary).Padding(0, 1)
	title := titleStyle.Render(" CAMERA CONFIGURATION ")

	filePath := "config.yaml"
	if m.appConfig != nil && m.appConfig.FilePath() != "" {
		filePath = m.appConfig.FilePath()
	}
	subtitle := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("Target file: %s", filePath))

	nameLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render("Camera Name / Alias (Optional):")
	nameInput := m.configInputs[0].View()

	targetLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render("Camera Target or IP:")
	targetHelp := lipgloss.NewStyle().Foreground(ColorMuted).Render("  (Leave empty to set Default credentials for all cameras)")
	targetInput := m.configInputs[1].View()

	userLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render("Username:")
	userInput := m.configInputs[2].View()

	passLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render("Password:")
	passInput := m.configInputs[3].View()

	saveBtnStyle := lipgloss.NewStyle().Padding(0, 2).Bold(true)
	if m.configFocus == 4 {
		saveBtnStyle = saveBtnStyle.Foreground(lipgloss.Color("#FFFFFF")).Background(ColorActive)
	} else {
		saveBtnStyle = saveBtnStyle.Foreground(lipgloss.Color("#D1D5DB")).Background(lipgloss.Color("#374151"))
	}
	saveBtn := saveBtnStyle.Render("[ SAVE TO CONFIG.YAML ]")

	hints := lipgloss.NewStyle().Foreground(ColorMuted).Render("Enter: Save | Tab/↓: Next Field | Shift+Tab/↑: Prev | Esc: Close")

	var msgView string
	if m.configMessage != "" {
		if m.configErr {
			msgView = lipgloss.NewStyle().Foreground(ColorDanger).Bold(true).Render(m.configMessage)
		} else {
			msgView = lipgloss.NewStyle().Foreground(ColorActive).Bold(true).Render(m.configMessage)
		}
	}

	elements := []string{
		lipgloss.JoinHorizontal(lipgloss.Center, title, " ", subtitle),
		"",
		nameLabel,
		"  " + nameInput,
		"",
		targetLabel,
		targetHelp,
		"  " + targetInput,
		"",
		userLabel,
		"  " + userInput,
		"",
		passLabel,
		"  " + passInput,
		"",
	}
	if msgView != "" {
		elements = append(elements, msgView, "")
	}
	elements = append(elements,
		lipgloss.JoinHorizontal(lipgloss.Center, saveBtn),
		"",
		lipgloss.JoinHorizontal(lipgloss.Center, hints),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, elements...)

	dialogBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorDarkBg).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialogBox)
}

func truncate(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func shortenTopic(topic string) string {
	// Strip namespace prefix e.g. "tns1:"
	if idx := strings.Index(topic, ":"); idx != -1 {
		topic = topic[idx+1:]
	}
	return topic
}
