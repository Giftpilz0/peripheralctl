package kbdtui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

const (
	lightBrightness = iota
	lightEffect
	lightSpeed
	lightColor
)

// lightRows lists the settings this keyboard has.
func (m *model) lightRows() []int {
	menu := m.def.Menu
	if menu == nil {
		return nil
	}
	var rows []int
	for _, r := range []struct {
		id  byte
		row int
	}{{menu.Brightness, lightBrightness}, {menu.Effect, lightEffect}, {menu.Speed, lightSpeed}, {menu.Color, lightColor}} {
		if r.id != 0 {
			rows = append(rows, r.row)
		}
	}
	return rows
}

func (m *model) updateLighting(key string) {
	rows := m.lightRows()
	if m.state == nil || len(rows) == 0 {
		return
	}
	step := 0
	switch key {
	case "up", "k":
		m.lightCursor = max(0, m.lightCursor-1)
		return
	case "down", "j":
		m.lightCursor = min(len(rows)-1, m.lightCursor+1)
		return
	case "left", "h":
		step = -1
	case "right", "l":
		step = 1
	case "enter":
	default:
		return
	}
	if m.readOnly() {
		m.refuseWrite()
		return
	}
	row := rows[min(m.lightCursor, len(rows)-1)]
	if step == 0 {
		if row == lightColor {
			m.color = colorPickerFor(m.light.Color())
		}
		return
	}
	l := &m.light
	switch row {
	case lightBrightness:
		l.Brightness = byte(max(0, min(m.def.MaxBrightness, int(l.Brightness)+step)))
	case lightEffect:
		if n := len(m.def.Effects); n > 0 {
			l.Effect = byte((int(l.Effect) + step + n) % n)
		}
	case lightSpeed:
		l.Speed = byte(max(0, min(m.def.MaxSpeed, int(l.Speed)+step)))
	}
}

func effectName(def *via.Definition, e byte) string {
	if int(e) < len(def.Effects) {
		return def.Effects[e]
	}
	return fmt.Sprintf("effect %d", e)
}

func meter(v, top int) string {
	const width = 20
	fill := v * width / max(1, top)
	return strings.Repeat("█", fill) + theme.Dim.Render(strings.Repeat("░", width-fill))
}

func (m *model) lightingView() string {
	if m.state == nil {
		return theme.Dim.Render("the keyboard has not been read")
	}
	if m.color != nil {
		return theme.Title.Render(m.color.Title) + "\n\n" + m.color.View()
	}
	shown := m.lightRows()
	if len(shown) == 0 {
		return theme.Dim.Render("this keyboard's definition has no lighting controls")
	}
	l := m.light
	effect := effectName(m.def, l.Effect)
	values := map[int]struct{ label, value string }{
		lightBrightness: {"Brightness", fmt.Sprintf("%s %d", meter(int(l.Brightness), m.def.MaxBrightness), l.Brightness)},
		lightEffect:     {"Effect", fmt.Sprintf("‹ %s ›", effect)},
		lightSpeed:      {"Speed", fmt.Sprintf("%s %d", meter(int(l.Speed), m.def.MaxSpeed), l.Speed)},
		lightColor:      {"Colour", fmt.Sprintf("%s %s  %s", colorSwatch(l.Color()), colorRGB(l.Color()), theme.Dim.Render("enter to pick"))},
	}
	var b strings.Builder
	for i, row := range shown {
		r := values[row]
		label := fmt.Sprintf("%-11s", r.label)
		if i == m.lightCursor {
			b.WriteString(theme.Title.Render("› "+label) + " " + r.value + "\n")
		} else {
			b.WriteString("  " + label + " " + r.value + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim.Render("  Changes reach the keyboard when applied.") + "\n\n")
	if leds := m.ledShortcuts(); len(leds) > 0 {
		b.WriteString(theme.Title.Render("  Logo and side LEDs") + "\n")
		for _, s := range leds {
			fmt.Fprintf(&b, "  %-13s %s\n", s.label, s.keys)
		}
		b.WriteString(theme.Dim.Render("  The firmware only has keys for these, with no state to read or set. Assign them under Keymap."))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ledFunctions are the Crush 80 custom keycodes for LEDs the VIA commands don't
// reach. A keyboard without them shows no panel.
var ledFunctions = []struct{ label, name string }{
	{"Logo effect", "LogoM"},
	{"Logo colour", "LogoC"},
	{"Logo on/off", "LogoP"},
	{"Side effect", "SideM"},
	{"Side colour", "SideC"},
	{"Side on/off", "SideP"},
	{"All LEDs", "LEDP"},
}

type ledShortcut struct{ label, keys string }

// ledShortcuts finds which keys hold each LED function, staged edits included.
func (m *model) ledShortcuts() []ledShortcut {
	out := make([]ledShortcut, 0, len(ledFunctions))
	for _, f := range ledFunctions {
		if !slices.Contains(m.def.Custom, f.name) {
			continue
		}
		var keys []string
		for layer := range m.state.Layers {
			for _, row := range m.def.Keys {
				for _, k := range row {
					code := m.state.At(layer, k.Row, k.Col)
					if edit, ok := m.edits[keyPos{layer, k.Row, k.Col}]; ok {
						code = edit
					}
					if via.Label(m.state.Protocol, code, m.def.Custom) != f.name {
						continue
					}
					switch layer {
					case 0:
						keys = append(keys, k.Stock)
					case 1:
						keys = append(keys, "Fn+"+k.Stock)
					default:
						keys = append(keys, fmt.Sprintf("Layer %d: %s", layer, k.Stock))
					}
				}
			}
		}
		if len(keys) == 0 {
			keys = []string{"no key assigned"}
		}
		out = append(out, ledShortcut{f.label, strings.Join(keys, ", ")})
	}
	return out
}
