package kbdtui

import (
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/giftpilz0/peripheralctl/internal/colorpick"
	"github.com/giftpilz0/peripheralctl/internal/razer"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// A drag makes one write: the colour goes out once the slider rests this long.
// Whether the firmware keeps these writes out of flash is unknown.
const previewPause = 300 * time.Millisecond

type previewTickMsg int

type previewDoneMsg struct {
	color via.Color
	err   error
}

func colorPickerFor(c via.Color) *colorpick.Picker {
	return colorpick.NewHueSat("Backlight colour", float64(c.Hue)*360/255, float64(c.Saturation)/255)
}

func pickedColor(p *colorpick.Picker) via.Color {
	h, s := p.HueSat()
	return via.Color{Hue: byte(int(math.Round(h*255/360)) % 256), Saturation: byte(math.Round(s * 255))}
}

func colorRGB(c via.Color) razer.RGB {
	return colorpick.HSVToRGB(float64(c.Hue)*360/255, float64(c.Saturation)/255, 1)
}

func colorSwatch(c via.Color) string {
	return lipgloss.NewStyle().Background(lipgloss.Color(colorRGB(c).String())).Render("    ")
}

func (m *model) updateColor(msg tea.KeyMsg) tea.Cmd {
	act, changed := m.color.Update(msg)
	switch act {
	case colorpick.Confirm:
		c := pickedColor(m.color)
		m.light.Hue, m.light.Saturation = c.Hue, c.Saturation
		m.color = nil
		m.previewSeq++
		m.ledWant = c
		return m.syncLED()
	case colorpick.Cancel:
		m.color = nil
		m.previewSeq++
		m.ledWant = m.light.Color()
		return m.syncLED()
	}
	if !changed {
		return nil
	}
	m.previewSeq++
	seq := m.previewSeq
	return tea.Tick(previewPause, func(time.Time) tea.Msg { return previewTickMsg(seq) })
}

// syncLED sends the wanted colour unless it shows already. One write runs at a
// time; previewDone sends whatever became wanted meanwhile.
func (m *model) syncLED() tea.Cmd {
	if m.dev == nil || m.previewBusy || m.ledShown == m.ledWant {
		return nil
	}
	m.previewBusy = true
	dev, c := m.dev, m.ledWant
	return func() tea.Msg { return previewDoneMsg{c, dev.Preview(c)} }
}

func (m *model) previewDone(msg previewDoneMsg) tea.Cmd {
	m.previewBusy = false
	if msg.err != nil {
		m.err = fmt.Errorf("colour preview: %w", msg.err)
		m.ledWant = m.ledShown // no retry loop
		m.afterLED = nil
		return nil
	}
	m.ledShown = msg.color
	if m.ledShown == m.ledWant && m.afterLED != nil {
		then := m.afterLED
		m.afterLED = nil
		return then()
	}
	return m.syncLED()
}

// savedColor is the colour the keyboard had before a preview changed it.
func (m *model) savedColor() *via.Color {
	if c := m.state.Lighting.Color(); m.ledShown != c {
		return &c
	}
	return nil
}
