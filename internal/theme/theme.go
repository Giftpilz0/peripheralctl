// Package theme holds the colours and styles shared by the screens.
package theme

import "github.com/charmbracelet/lipgloss"

var (
	AccentColor = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#44d17a"}
	DimColor    = lipgloss.AdaptiveColor{Light: "#6e7781", Dark: "#8b949e"}
	WarnColor   = lipgloss.AdaptiveColor{Light: "#bc4c00", Dark: "#f0883e"}
	BadColor    = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#ff7b72"}

	Title = lipgloss.NewStyle().Bold(true).Foreground(AccentColor)
	Dim   = lipgloss.NewStyle().Foreground(DimColor)
	Warn  = lipgloss.NewStyle().Foreground(WarnColor)
	Bad   = lipgloss.NewStyle().Foreground(BadColor)
	Tab   = lipgloss.NewStyle().Padding(0, 1).Foreground(DimColor)
	TabOn = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(AccentColor).Underline(true)
)
