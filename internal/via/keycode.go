package via

import (
	"fmt"
	"strings"
)

// ranges holds the keycode ranges that moved between VIA protocol versions.
// Values come from the-via/app src/utils/key-to-byte.
type ranges struct {
	modTap, modTapMax, layerMod, layerModMax    uint16
	to, momentary, defLayer, toggle, oneShot    uint16
	layerSpan                                   uint16 // size of the five ranges above
	oneShotMod, oneShotModMax, tapToggle        uint16
	layerModBits                                uint
	macro, macroMax, custom, customMax, rgbBase uint16
	boot, clearEEPROM, graveEsc                 uint16
	mouseBase                                   uint16 // KC_MS_UP
}

var (
	rangesLegacy = ranges{
		modTap: 0x6000, modTapMax: 0x7fff, layerMod: 0x5900, layerModMax: 0x59ff,
		to: 0x5010, momentary: 0x5100, defLayer: 0x5200, toggle: 0x5300, oneShot: 0x5400, layerSpan: 0x20,
		oneShotMod: 0x5500, oneShotModMax: 0x55ff, tapToggle: 0x5800, layerModBits: 4,
		macro: 0x5f12, macroMax: 0x5f21, custom: 0x5f80, customMax: 0x5f8f, rgbBase: 0x5cc2,
		boot: 0x5c00, graveEsc: 0x5c16, mouseBase: 0x00f0,
	}
	ranges10 = func() ranges {
		r := rangesLegacy
		r.to = 0x5000
		return r
	}()
	ranges11 = ranges{
		modTap: 0x2000, modTapMax: 0x3fff, layerMod: 0x5000, layerModMax: 0x51ff,
		to: 0x5200, momentary: 0x5220, defLayer: 0x5240, toggle: 0x5260, oneShot: 0x5280, layerSpan: 0x20,
		oneShotMod: 0x52a0, oneShotModMax: 0x52bf, tapToggle: 0x52c0, layerModBits: 5,
		macro: 0x7702, macroMax: 0x7711, custom: 0x7f00, customMax: 0x7fff, rgbBase: 0x7820,
		boot: 0x7c00, clearEEPROM: 0x7c03, graveEsc: 0x7c16, mouseBase: 0x00cd,
	}
	ranges12 = func() ranges {
		r := ranges11
		r.macro, r.macroMax, r.custom, r.customMax = 0x7700, 0x777f, 0x7e00, 0x7eff
		return r
	}()
)

func rangesFor(protocol int) (ranges, bool) {
	switch {
	case protocol <= 9:
		return rangesLegacy, true
	case protocol == 10:
		return ranges10, true
	case protocol == 11:
		return ranges11, true
	case protocol == 12:
		return ranges12, true
	}
	return ranges{}, false // newer tables depend on the QMK keycodes version
}

var basicNames = func() map[uint16]string {
	m := map[uint16]string{
		0x00: "", 0x01: "▽",
		0x28: "Enter", 0x29: "Esc", 0x2a: "Bksp", 0x2b: "Tab", 0x2c: "Space",
		0x2d: "-", 0x2e: "=", 0x2f: "[", 0x30: "]", 0x31: "\\", 0x32: "#",
		0x33: ";", 0x34: "'", 0x35: "`", 0x36: ",", 0x37: ".", 0x38: "/", 0x39: "Caps",
		0x46: "PrtSc", 0x47: "ScrLk", 0x48: "Pause", 0x49: "Ins", 0x4a: "Home", 0x4b: "PgUp",
		0x4c: "Del", 0x4d: "End", 0x4e: "PgDn", 0x4f: "→", 0x50: "←", 0x51: "↓", 0x52: "↑",
		0x53: "NumLk", 0x54: "P/", 0x55: "P*", 0x56: "P-", 0x57: "P+", 0x58: "PEnt",
		0x62: "P0", 0x63: "P.", 0x64: "ISO\\", 0x65: "Menu", 0x66: "Power", 0x67: "P=",
		0xa5: "PwrOff", 0xa6: "Sleep", 0xa7: "Wake", 0xa8: "Mute", 0xa9: "Vol+", 0xaa: "Vol-",
		0xab: "Next", 0xac: "Prev", 0xad: "Stop", 0xae: "Play", 0xaf: "Media", 0xb0: "Eject",
		0xb1: "Mail", 0xb2: "Calc", 0xb3: "MyPC", 0xb4: "Search", 0xb5: "WebHome", 0xb6: "Back",
		0xb7: "Fwd", 0xb8: "WebStop", 0xb9: "Reload", 0xba: "Favs", 0xbb: "FFwd", 0xbc: "Rew",
		0xbd: "Bri+", 0xbe: "Bri-",
		0xe0: "LCtrl", 0xe1: "LShft", 0xe2: "LAlt", 0xe3: "LWin",
		0xe4: "RCtrl", 0xe5: "RShft", 0xe6: "RAlt", 0xe7: "RWin",
	}
	for i := range uint16(26) {
		m[0x04+i] = string(rune('A' + i))
	}
	for i := range uint16(9) {
		m[0x1e+i] = string(rune('1' + i))
		m[0x59+i] = fmt.Sprintf("P%d", i+1)
	}
	m[0x27] = "0"
	for i := range uint16(12) {
		m[0x3a+i] = fmt.Sprintf("F%d", i+1)
		m[0x68+i] = fmt.Sprintf("F%d", i+13)
	}
	return m
}()

var mouseNames = []string{
	"MsUp", "MsDn", "MsLft", "MsRgt", "MB1", "MB2", "MB3", "MB4", "MB5",
}

var rgbNames = []string{
	"RGB", "RGB+", "RGB-", "Hue+", "Hue-", "Sat+", "Sat-", "Val+", "Val-", "Spd+", "Spd-",
}

// firmwareKey is not a VIA keycode. The Crush 80 stores it on keys its firmware
// seems to handle itself, such as the Fn+F1 media functions.
const firmwareKey = 0xffff

func hexCode(code uint16) string { return fmt.Sprintf("%#04x", code) }

func modMask(m uint16) string {
	prefix := ""
	if m&0x10 != 0 {
		prefix = "R"
	}
	var out []string
	for i, name := range []string{"C", "S", "A", "G"} {
		if m&(1<<i) != 0 {
			out = append(out, prefix+name)
		}
	}
	return strings.Join(out, "+")
}

func basic(protocol int, code uint16) string {
	if name, ok := basicNames[code]; ok {
		return name
	}
	if r, ok := rangesFor(protocol); ok && code >= r.mouseBase && code-r.mouseBase < uint16(len(mouseNames)) {
		return mouseNames[code-r.mouseBase]
	}
	return hexCode(code)
}

// Label names a keycode for a protocol version; custom holds the keyboard's
// custom keycode names. Keycodes it does not know come back as hex.
func Label(protocol int, code uint16, custom []string) string {
	if code == firmwareKey {
		return "fw"
	}
	if code < 0x100 {
		return basic(protocol, code)
	}
	if code <= 0x1fff {
		return modMask(code>>8&0x1f) + "+" + basic(protocol, code&0xff)
	}
	if code >= 0x4000 && code <= 0x4fff {
		return fmt.Sprintf("LT%d %s", code>>8&0xf, basic(protocol, code&0xff))
	}
	r, ok := rangesFor(protocol)
	if !ok {
		return hexCode(code)
	}
	inLayer := func(base uint16) bool { return code >= base && code < base+r.layerSpan }
	switch {
	case code >= r.modTap && code <= r.modTapMax:
		return fmt.Sprintf("%s/%s", modMask(code>>8&0x1f), basic(protocol, code&0xff))
	case code >= r.layerMod && code <= r.layerModMax:
		rem := code - r.layerMod
		return fmt.Sprintf("LM%d %s", rem>>r.layerModBits, modMask(rem&(1<<r.layerModBits-1)))
	case inLayer(r.to):
		return fmt.Sprintf("TO%d", code-r.to)
	case inLayer(r.momentary):
		return fmt.Sprintf("MO%d", code-r.momentary)
	case inLayer(r.defLayer):
		return fmt.Sprintf("DF%d", code-r.defLayer)
	case inLayer(r.toggle):
		return fmt.Sprintf("TG%d", code-r.toggle)
	case inLayer(r.oneShot):
		return fmt.Sprintf("OSL%d", code-r.oneShot)
	case inLayer(r.tapToggle):
		return fmt.Sprintf("TT%d", code-r.tapToggle)
	case code >= r.oneShotMod && code <= r.oneShotModMax:
		return "OSM " + modMask(code-r.oneShotMod)
	case code >= r.macro && code <= r.macroMax:
		return fmt.Sprintf("M%d", code-r.macro)
	case code >= r.custom && code <= r.customMax:
		if i := int(code - r.custom); i < len(custom) {
			return custom[i]
		}
		return fmt.Sprintf("Custom%d", code-r.custom)
	case code >= r.rgbBase && code-r.rgbBase < uint16(len(rgbNames)):
		return rgbNames[code-r.rgbBase]
	case code == r.boot:
		return "Boot"
	case code == r.graveEsc:
		return "GEsc"
	}
	return hexCode(code)
}

// Entry is one keycode that can be assigned to a key.
type Entry struct {
	Code uint16
	Name string
}

// Catalog lists the assignable keycodes. Bootloader and EEPROM keycodes are
// left out; see Dangerous.
func Catalog(protocol, layers int, custom []string) []Entry {
	var out []Entry
	add := func(code uint16) {
		if name := Label(protocol, code, custom); name != "" && name != "▽" && !strings.HasPrefix(name, "0x") {
			out = append(out, Entry{code, name})
		}
	}
	out = append(out, Entry{0x0000, "None"}, Entry{0x0001, "▽ Transparent"})
	for code := uint16(0x02); code < 0x100; code++ {
		add(code)
	}
	r, ok := rangesFor(protocol)
	if !ok {
		return out
	}
	for layer := range uint16(layers) {
		for _, base := range []uint16{r.momentary, r.toggle, r.to, r.defLayer, r.oneShot, r.tapToggle} {
			add(base + layer)
		}
	}
	for i := range uint16(len(rgbNames)) {
		add(r.rgbBase + i)
	}
	for i := range uint16(len(custom)) {
		add(r.custom + i)
	}
	return out
}

// Dangerous tells whether assigning code could wipe the keyboard or leave it in
// the bootloader on a keypress. On an unknown keycode table, everything beyond
// plain keys counts.
func Dangerous(protocol int, code uint16) bool {
	r, ok := rangesFor(protocol)
	if !ok {
		return code > 0x1fff
	}
	return code == r.boot || (r.clearEEPROM != 0 && code == r.clearEEPROM)
}
