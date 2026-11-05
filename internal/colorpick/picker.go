// Package colorpick is the colour picker shared by the mouse and keyboard screens.
package colorpick

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/giftpilz0/peripheralctl/internal/razer"
	"github.com/giftpilz0/peripheralctl/internal/theme"
)

// palette holds two rows of LED-friendly colours. LEDs render saturated
// colours well and pastels poorly, so most entries are fully saturated.
var palette = [2][10]razer.RGB{
	hexRow("ff0000", "ff4000", "ff8c00", "ffd200", "a0ff00", "00ff00", "00ff78", "00ffff", "008cff", "0000ff"),
	hexRow("5a00ff", "a000ff", "ff00ff", "ff008c", "ff003c", "44d62c", "ffffff", "ffbe78", "ff783c", "282828"),
}

func hexRow(hex ...string) (row [10]razer.RGB) {
	for i, h := range hex {
		c, err := razer.ParseRGB(h)
		if err != nil {
			panic(err)
		}
		row[i] = c
	}
	return row
}

const (
	rowPalette1 = iota
	rowPalette2
	rowHue
	rowSat
	rowVal
	pickerRows
)

const barWidth = 40

// Action is what a key press asks the caller to do.
type Action int

const (
	None Action = iota
	Confirm
	Cancel
)

// Picker edits one colour with a palette and HSV sliders. Hue, saturation
// and value are kept separately so hue survives passing through grey.
type Picker struct {
	Title   string
	Col     razer.RGB
	h, s, v float64 // 0..360, 0..1, 0..1
	hueSat  bool    // no value slider: the device sets brightness elsewhere
	row     int
	pal     int
	hexMode bool
	input   textinput.Model
}

// New edits a full colour.
func New(title string, c razer.RGB) *Picker {
	p := blank(title)
	p.setRGB(c)
	p.findPalette()
	return p
}

// NewHueSat edits only hue (0..360) and saturation (0..1), for a device whose
// brightness is a separate setting. An untouched picker keeps the exact values.
func NewHueSat(title string, h, s float64) *Picker {
	p := blank(title)
	p.hueSat = true
	p.h, p.s, p.v = h, s, 1
	p.setHSV()
	p.findPalette()
	return p
}

func blank(title string) *Picker {
	p := &Picker{Title: title, input: textinput.New(), row: rowHue}
	p.input.CharLimit = 7
	p.input.Prompt = "#"
	return p
}

// findPalette starts on the palette entry that matches, if any.
func (p *Picker) findPalette() {
	for r := range palette {
		for i, pc := range palette[r] {
			if pc == p.Col {
				p.row, p.pal = r, i
			}
		}
	}
}

// HueSat returns the hue in degrees and the saturation.
func (p *Picker) HueSat() (h, s float64) { return p.h, p.s }

func (p *Picker) setRGB(c razer.RGB) {
	p.Col = c
	h, s, v := RGBToHSV(c)
	if s > 0 && v > 0 {
		p.h = h
	}
	if v > 0 {
		p.s = s
	}
	p.v = v
	if p.hueSat {
		p.v = 1
		p.setHSV()
	}
}

func (p *Picker) setHSV() { p.Col = HSVToRGB(p.h, p.s, p.v) }

func (p *Picker) rows() int {
	if p.hueSat {
		return rowVal
	}
	return pickerRows
}

// Paste inserts pasted text into the hex field while it is open.
func (p *Picker) Paste(msg tea.PasteMsg) {
	if p.hexMode {
		p.input, _ = p.input.Update(msg)
	}
}

// Update handles a key. changed reports a new colour, for live preview.
func (p *Picker) Update(k tea.KeyPressMsg) (act Action, changed bool) {
	if p.hexMode {
		switch k.String() {
		case "esc":
			p.hexMode = false
		case "enter":
			c, err := razer.ParseRGB(p.input.Value())
			if err != nil {
				return None, false
			}
			p.hexMode = false
			p.setRGB(c)
			return None, true
		default:
			p.input, _ = p.input.Update(k)
		}
		return None, false
	}

	old := p.Col
	key := k.String()
	switch key {
	case "esc", "q":
		return Cancel, false
	case "enter", "space":
		return Confirm, false
	case "#", "x":
		p.hexMode = true
		p.input.SetValue(strings.TrimPrefix(p.Col.String(), "#"))
		p.input.CursorEnd()
		p.input.Focus()
		return None, false
	case "up", "k":
		p.row = (p.row + p.rows() - 1) % p.rows()
	case "down", "j":
		p.row = (p.row + 1) % p.rows()
	case "left", "h", "right", "l", "shift+left", "shift+right", "pgup", "pgdown", "H", "L":
		dir := 1.0
		if key == "left" || key == "h" || key == "shift+left" || key == "pgdown" || key == "H" {
			dir = -1
		}
		big := key == "shift+left" || key == "shift+right" || key == "pgup" || key == "pgdown" || key == "H" || key == "L"
		p.step(dir, big)
	case "home":
		p.step(-1000, false)
	case "end":
		p.step(1000, false)
	}
	return None, p.Col != old
}

func (p *Picker) step(dir float64, big bool) {
	switch p.row {
	case rowPalette1, rowPalette2:
		n := len(palette[p.row])
		p.pal = max(0, min(n-1, p.pal+int(dir)))
		p.setRGB(palette[p.row][p.pal])
	case rowHue:
		d := 5.0
		if big {
			d = 30
		}
		p.h = math.Mod(p.h+dir*d+3600, 360)
		if math.Abs(dir) > 1 {
			p.h = map[bool]float64{true: 359, false: 0}[dir > 0]
		}
		p.setHSV()
	case rowSat, rowVal:
		d := 0.02
		if big {
			d = 0.1
		}
		x := &p.s
		if p.row == rowVal {
			x = &p.v
		}
		*x = math.Max(0, math.Min(1, *x+dir*d))
		p.setHSV()
	}
}

// ---- rendering ----

func cell(c razer.RGB, text string) string {
	fg := lipgloss.Color("#000000")
	if luminance(c) < 0.5 {
		fg = lipgloss.Color("#ffffff")
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(c.String())).Foreground(fg).Render(text)
}

func luminance(c razer.RGB) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// bar draws a gradient; at(x) gives the colour for x in [0,1], pos marks
// the current value.
func bar(at func(x float64) razer.RGB, pos float64) string {
	var b strings.Builder
	mark := int(math.Round(pos * (barWidth - 1)))
	for i := 0; i < barWidth; i++ {
		x := float64(i) / (barWidth - 1)
		t := " "
		if i == mark {
			t = "┃"
		}
		b.WriteString(cell(at(x), t))
	}
	return b.String()
}

// View renders the picker.
func (p *Picker) View() string {
	var b strings.Builder
	label := func(row int, s string) string {
		if row == p.row && !p.hexMode {
			return theme.Title.Render("› " + fmt.Sprintf("%-6s", s))
		}
		return "  " + theme.Dim.Render(fmt.Sprintf("%-6s", s))
	}

	for r := range palette {
		name := ""
		if r == 0 {
			name = "Preset"
		}
		b.WriteString(label(r, name))
		for i, c := range palette[r] {
			t := "   "
			if c == p.Col {
				t = " ● "
			}
			if r == p.row && i == p.pal && !p.hexMode {
				t = "[" + strings.TrimSpace(t) + "]"
				if len(t) == 2 {
					t = "[ ]"
				}
			}
			b.WriteString(cell(c, t) + " ")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(label(rowHue, "Hue") + bar(func(x float64) razer.RGB { return HSVToRGB(x*360, 1, 1) }, p.h/360) +
		theme.Dim.Render(fmt.Sprintf(" %3.0f°", p.h)) + "\n")
	b.WriteString(label(rowSat, "Sat") + bar(func(x float64) razer.RGB { return HSVToRGB(p.h, x, math.Max(p.v, 0.25)) }, p.s) +
		theme.Dim.Render(fmt.Sprintf(" %3.0f%%", p.s*100)) + "\n")
	if !p.hueSat {
		b.WriteString(label(rowVal, "Value") + bar(func(x float64) razer.RGB { return HSVToRGB(p.h, p.s, x) }, p.v) +
			theme.Dim.Render(fmt.Sprintf(" %3.0f%%", p.v*100)) + "\n")
	}
	b.WriteString("\n")

	sw := cell(p.Col, strings.Repeat(" ", 12))
	hex := p.Col.String()
	if p.hexMode {
		hex = p.input.View()
	}
	b.WriteString("  " + sw + "  " + hex + "\n")
	b.WriteString("  " + sw + "  " + theme.Dim.Render(fmt.Sprintf("rgb(%d, %d, %d)", p.Col.R, p.Col.G, p.Col.B)))
	return b.String()
}

// Help describes the keys.
func (p *Picker) Help() string {
	if p.hexMode {
		return "type a hex colour · enter set · esc back"
	}
	return "↑↓ row · ←→ adjust (shift+←→ bigger steps) · # hex · enter choose · esc cancel"
}

// ---- colour math ----

// HSVToRGB converts hue in degrees, saturation and value in 0..1.
func HSVToRGB(h, s, v float64) razer.RGB {
	h = math.Mod(h, 360)
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to := func(f float64) uint8 { return uint8(math.Round((f + m) * 255)) }
	return razer.RGB{R: to(r), G: to(g), B: to(b)}
}

// RGBToHSV is the inverse of HSVToRGB.
func RGBToHSV(c razer.RGB) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := mx - mn
	v = mx
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}
