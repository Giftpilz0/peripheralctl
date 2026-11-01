package kbdtui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

const pickerRows = 8

// picker chooses a keycode for one key on the current layer.
type picker struct {
	target   via.Key
	protocol int
	custom   []string
	all      []via.Entry
	matches  []via.Entry
	cursor   int
	input    textinput.Model
	problem  string
}

func newPicker(s *via.State, def *via.Definition, target via.Key) *picker {
	in := textinput.New()
	in.Placeholder = "A, F13, Vol+, MO1, RGB, 0x0104…"
	in.Focus()
	p := &picker{
		target:   target,
		protocol: s.Protocol,
		custom:   def.Custom,
		all:      via.Catalog(s.Protocol, s.Layers, def.Custom),
		input:    in,
	}
	p.filter()
	return p
}

// rank orders exact matches first, then prefixes, then the rest; -1 is no match.
func rank(name, query string) int {
	name = strings.ToLower(name)
	switch {
	case name == query:
		return 0
	case strings.HasPrefix(name, query):
		return 1
	case strings.Contains(name, query):
		return 2
	}
	return -1
}

func (p *picker) filter() {
	q := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.matches = p.matches[:0]
	for _, e := range p.all {
		if rank(e.Name, q) >= 0 {
			p.matches = append(p.matches, e)
		}
	}
	slices.SortStableFunc(p.matches, func(a, b via.Entry) int { return rank(a.Name, q) - rank(b.Name, q) })
	p.cursor = 0
}

// parseCode reads a raw keycode typed as hex, for those the catalog lacks.
func parseCode(s string) (uint16, bool) {
	hex, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	if !ok || hex == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 16)
	return uint16(v), err == nil
}

func (m *model) updatePicker(msg tea.KeyMsg) {
	p := m.picker
	switch msg.String() {
	case "esc":
		m.picker = nil
		return
	case "up":
		p.cursor = max(0, p.cursor-1)
		return
	case "down":
		p.cursor = min(max(0, len(p.matches)-1), p.cursor+1)
		return
	case "enter":
		code, ok := parseCode(p.input.Value())
		if !ok {
			if len(p.matches) == 0 {
				return
			}
			code = p.matches[p.cursor].Code
		}
		if via.Dangerous(p.protocol, code) {
			p.problem = fmt.Sprintf("%#04x could wipe the keyboard or strand it in the bootloader; not assignable here", code)
			return
		}
		pos := keyPos{m.layer, p.target.Row, p.target.Col}
		if m.state.At(pos.layer, pos.row, pos.col) == code {
			delete(m.edits, pos)
		} else {
			m.edits[pos] = code
		}
		m.picker = nil
		return
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	_ = cmd // only the blink, which the picker does not use
	p.problem = ""
	p.filter()
}

func (p *picker) view() string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("Assign to "+p.target.Stock) + "  " + p.input.View() + "\n")
	if code, ok := parseCode(p.input.Value()); ok {
		b.WriteString(theme.Dim.Render(fmt.Sprintf("raw keycode %#04x: %s", code, via.Label(p.protocol, code, p.custom))) + "\n")
	}
	if p.problem != "" {
		b.WriteString(theme.Bad.Render(p.problem) + "\n")
	}
	if len(p.matches) == 0 {
		b.WriteString(theme.Dim.Render("no matching keycode") + "\n")
	}
	start := max(0, min(p.cursor-pickerRows/2, len(p.matches)-pickerRows))
	for i := start; i < min(start+pickerRows, len(p.matches)); i++ {
		e := p.matches[i]
		line := fmt.Sprintf("%-16s %s", e.Name, theme.Dim.Render(fmt.Sprintf("%#04x", e.Code)))
		if i == p.cursor {
			line = theme.Title.Render("› ") + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
