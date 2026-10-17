package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Modern, refined Palette
	ColorPrimary   = lipgloss.Color("#818CF8") // Indigo-400
	ColorPrimaryBg = lipgloss.Color("#312E81") // Indigo-900
	ColorSecondary = lipgloss.Color("#38BDF8") // Sky-400
	ColorActive    = lipgloss.Color("#34D399") // Emerald-400
	ColorActiveBg  = lipgloss.Color("#064E3B") // Emerald-950
	ColorWarning   = lipgloss.Color("#FBBF24") // Amber-400
	ColorWarningBg = lipgloss.Color("#78350F") // Amber-950
	ColorDanger    = lipgloss.Color("#F87171") // Rose-400
	ColorDangerBg  = lipgloss.Color("#7F1D1D") // Rose-950
	ColorMuted     = lipgloss.Color("#94A3B8") // Slate-400
	ColorDim       = lipgloss.Color("#64748B") // Slate-500
	ColorBorder    = lipgloss.Color("#334155") // Slate-700
	ColorBorderAct = lipgloss.Color("#818CF8") // Indigo-400
	ColorHighlight = lipgloss.Color("#F472B6") // Pink-400
	ColorCyan      = lipgloss.Color("#22D3EE") // Cyan-400
	ColorDarkBg    = lipgloss.Color("#0F172A") // Slate-900
	ColorText      = lipgloss.Color("#F8FAFC") // Slate-50

	// Pane Borders
	StyleBorderNormal = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	StyleBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderAct).
				Padding(0, 1)

	// Titles
	StylePaneTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMuted)

	StylePaneTitleActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorPrimaryBg).
				Padding(0, 1)

	// Status Badges
	BadgeActive = lipgloss.NewStyle().
			Foreground(ColorActive).
			Bold(true)

	BadgeReconnecting = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)

	BadgeFailed = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	BadgeClosed = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Category Badges
	BadgeMotion = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#083344")).
			Background(ColorCyan).
			Bold(true).
			Padding(0, 1)

	BadgeInput = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1E1B4B")).
			Background(ColorPrimary).
			Bold(true).
			Padding(0, 1)

	BadgeTamper = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#450A0A")).
			Background(ColorDanger).
			Bold(true).
			Padding(0, 1)

	BadgeAudio = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4A044E")).
			Background(ColorHighlight).
			Bold(true).
			Padding(0, 1)

	BadgeSystem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#78350F")).
			Background(ColorWarning).
			Bold(true).
			Padding(0, 1)

	BadgeOther = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorDim).
			Padding(0, 1)

	// List row styles
	StyleRowSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("#2563EB")). // High-contrast Royal Blue
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	StyleDeviceRowSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("#2563EB")). // High-contrast Royal Blue
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	StyleRowFiltered = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E293B")). // Slate-800
				Foreground(lipgloss.Color("#38BDF8")). // Sky-400
				Bold(true)

	StyleRowNormal = lipgloss.NewStyle().
			Foreground(ColorText)

	// Header & Status bar
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#0F172A")).
			Padding(0, 1)

	StyleHelp = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Background(lipgloss.Color("#0F172A")).
			Padding(0, 1)

	StylePausedBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(ColorWarning).
				Padding(0, 1)
)

// RenderCategoryBadge returns formatted lipgloss badge for a category
func RenderCategoryBadge(cat string) string {
	switch cat {
	case "Motion":
		return BadgeMotion.Render("◈ MOTION")
	case "Input":
		return BadgeInput.Render("⇲ INPUT")
	case "Tamper":
		return BadgeTamper.Render("⚠ TAMPER")
	case "Audio":
		return BadgeAudio.Render("♪ AUDIO")
	case "System":
		return BadgeSystem.Render("⚙ SYSTEM")
	default:
		return BadgeOther.Render(cat)
	}
}

// RenderStateBadge returns formatted lipgloss badge for subscription state
func RenderStateBadge(state string) string {
	switch state {
	case "Active":
		return BadgeActive.Render("● Active")
	case "Active (Push webhook)":
		return BadgeActive.Render("● Push")
	case "Reconnecting":
		return BadgeReconnecting.Render("◐ Connecting")
	case "Failed":
		return BadgeFailed.Render("✕ Failed")
	default:
		if state == "" {
			state = "Inactive"
		}
		return BadgeClosed.Render("○ " + state)
	}
}
