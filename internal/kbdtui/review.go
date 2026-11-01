package kbdtui

import (
	"fmt"
	"strings"

	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// describe lists what writing want over cur changes, in the user's terms.
func describe(def *via.Definition, cur, want *via.State) []string {
	var out []string
	a, b := cur.Lighting, want.Lighting
	number := func(name string, from, to byte) {
		if from != to {
			out = append(out, fmt.Sprintf("%s: %d → %d", name, from, to))
		}
	}
	number("Brightness", a.Brightness, b.Brightness)
	if a.Effect != b.Effect {
		out = append(out, fmt.Sprintf("Effect: %s → %s", effectName(def, a.Effect), effectName(def, b.Effect)))
	}
	number("Speed", a.Speed, b.Speed)
	if a.Color() != b.Color() {
		out = append(out, fmt.Sprintf("Colour: %s → %s (hue %d → %d, saturation %d → %d)",
			colorRGB(a.Color()), colorRGB(b.Color()), a.Hue, b.Hue, a.Saturation, b.Saturation))
	}
	names := map[[2]int]string{}
	for _, row := range def.Keys {
		for _, k := range row {
			names[[2]int{k.Row, k.Col}] = k.Stock
		}
	}
	label := func(code uint16) string {
		if s := via.Label(cur.Protocol, code, def.Custom); s != "" {
			return s
		}
		return "no action"
	}
	for layer := range want.Layers {
		for row := range want.Rows {
			for col := range want.Cols {
				from, to := cur.At(layer, row, col), want.At(layer, row, col)
				if from == to {
					continue
				}
				name, ok := names[[2]int{row, col}]
				if !ok {
					name = fmt.Sprintf("matrix %d,%d", row, col)
				}
				out = append(out, fmt.Sprintf("Layer %d · %s: %s → %s", layer, name, label(from), label(to)))
			}
		}
	}
	return out
}

func (m *model) reviewView() string {
	limit := 25
	if m.height > 12 {
		limit = m.height - 10
	}
	var b strings.Builder
	b.WriteString(theme.Title.Render("What will change") + "\n\n")
	for i, line := range m.review {
		if i == limit {
			fmt.Fprintf(&b, "  %s\n", theme.Dim.Render(fmt.Sprintf("…and %d more", len(m.review)-limit)))
			break
		}
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n" + theme.Dim.Render("The previous state is kept as profile "+autoProfile+"."))
	return b.String()
}
