// Package theme holds the colours and styles shared by the screens.
package theme

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	AccentColor = compat.AdaptiveColor{Light: lipgloss.Color("#1a7f37"), Dark: lipgloss.Color("#44d17a")}
	DimColor    = compat.AdaptiveColor{Light: lipgloss.Color("#6e7781"), Dark: lipgloss.Color("#8b949e")}
	WarnColor   = compat.AdaptiveColor{Light: lipgloss.Color("#bc4c00"), Dark: lipgloss.Color("#f0883e")}
	BadColor    = compat.AdaptiveColor{Light: lipgloss.Color("#cf222e"), Dark: lipgloss.Color("#ff7b72")}

	Title = lipgloss.NewStyle().Bold(true).Foreground(AccentColor)
	Dim   = lipgloss.NewStyle().Foreground(DimColor)
	Warn  = lipgloss.NewStyle().Foreground(WarnColor)
	Bad   = lipgloss.NewStyle().Foreground(BadColor)
	Tab   = lipgloss.NewStyle().Padding(0, 1).Foreground(DimColor)
	TabOn = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(AccentColor).Underline(true)
)
