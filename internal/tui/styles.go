package tui

import "charm.land/lipgloss/v2"

var (
	colorAccent  = lipgloss.Color("#A78BFA")
	colorMuted   = lipgloss.Color("#6B7280")
	colorFaint   = lipgloss.Color("#4B5563")
	colorInk     = lipgloss.Color("#E5E7EB")
	colorDone    = lipgloss.Color("#34D399")
	colorSurface = lipgloss.Color("#1F2937")
	colorHeader  = lipgloss.Color("#312E81")
	colorFooter  = lipgloss.Color("#111827")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInk).
			Background(colorHeader)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorFooter)

	columnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint).
			Padding(0, 1)

	columnFocusedStyle = columnStyle.
				BorderForeground(colorAccent)

	columnTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMuted)

	columnTitleFocusedStyle = columnTitleStyle.
				Foreground(colorAccent)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint).
			Padding(0, 1)

	cardSelectedStyle = cardStyle.
				BorderForeground(colorAccent)

	cardTitleStyle = lipgloss.NewStyle().
			Foreground(colorInk)

	cardTitleSelectedStyle = cardTitleStyle.
				Bold(true).
				Foreground(colorAccent)

	cardTitleDoneStyle = cardTitleStyle.
				Faint(true).
				Strikethrough(true)

	metaStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	doneDotStyle = lipgloss.NewStyle().
			Foreground(colorDone)

	helpBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 3)

	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInk)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 1)

	overlayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	overlaySelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	overlayHintStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	overlayErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F87171"))

	dimCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFaint).
			Padding(0, 1).
			Faint(true)

	grabColumnStyle = columnStyle.
			BorderForeground(colorDone)

	grabMarkerStyle = lipgloss.NewStyle().
			Foreground(colorDone).
			Bold(true)

	tabBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorFooter)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Background(colorFooter)
)
