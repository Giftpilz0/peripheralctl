package colorpick

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/giftpilz0/peripheralctl/internal/razer"
)

func TestHSVRoundTrip(t *testing.T) {
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 15 {
			for b := 0; b < 256; b += 15 {
				c := razer.RGB{R: uint8(r), G: uint8(g), B: uint8(b)}
				if got := HSVToRGB(RGBToHSV(c)); got != c {
					t.Fatalf("%s -> %s", c, got)
				}
			}
		}
	}
}

func TestPickerKeepsHueThroughGrey(t *testing.T) {
	p := New("Colour", razer.RGB{R: 255})
	p.row = rowVal
	p.step(-1000, false) // value 0: black
	p.step(+1000, false) // back to full value
	if p.Col != (razer.RGB{R: 255}) {
		t.Fatalf("hue lost: %s", p.Col)
	}
}

func TestPickerKeys(t *testing.T) {
	p := New("Colour", razer.RGB{G: 255}) // palette row 0, index 5
	if p.row != rowPalette1 || p.pal != 5 {
		t.Fatalf("start at row %d pal %d", p.row, p.pal)
	}
	if _, changed := p.Update(tea.KeyPressMsg{Code: tea.KeyRight}); !changed || p.Col != palette[0][6] {
		t.Fatalf("right: %s changed=%v", p.Col, changed)
	}
	p.Update(tea.KeyPressMsg{Code: '#', Text: "#"})
	p.input.SetValue("123456")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.Col != (razer.RGB{R: 0x12, G: 0x34, B: 0x56}) || p.hexMode {
		t.Fatalf("hex entry: %s hexMode=%v", p.Col, p.hexMode)
	}
	if act, _ := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); act != Confirm {
		t.Fatal("enter should confirm")
	}
}

func TestPickerPasteAndSpace(t *testing.T) {
	p := New("Colour", razer.RGB{G: 255})
	p.Update(tea.KeyPressMsg{Code: '#', Text: "#"})
	p.input.SetValue("")
	p.Paste(tea.PasteMsg{Content: "abcdef"})
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.Col != (razer.RGB{R: 0xab, G: 0xcd, B: 0xef}) {
		t.Fatalf("pasted hex: %s", p.Col)
	}
	if act, _ := p.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}); act != Confirm {
		t.Fatal("space should confirm")
	}
}

func TestHueSatPickerKeepsExactValues(t *testing.T) {
	p := NewHueSat("Colour", 240, 1)
	if p.Col != (razer.RGB{B: 255}) {
		t.Fatalf("colour %s", p.Col)
	}
	if h, s := p.HueSat(); h != 240 || s != 1 {
		t.Fatalf("hue %v saturation %v", h, s)
	}
}

func TestHueSatPickerHasNoValueRow(t *testing.T) {
	p := NewHueSat("Colour", 123, 0.5)
	p.row = rowSat
	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if p.row != rowPalette1 {
		t.Fatalf("down from saturation went to row %d", p.row)
	}
	p.setRGB(razer.RGB{R: 40, G: 40, B: 40}) // dark grey has no hue
	if p.v != 1 || p.h != 123 {
		t.Fatalf("value %v hue %v", p.v, p.h)
	}
}
