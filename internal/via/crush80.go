package via

// Crush80 is the Wobkey Crush 80, wired or through its 2.4G receiver.
var Crush80 = Definition{
	Name:     "Wobkey Crush 80",
	Vendor:   0x320f,
	Products: []uint16{0x5055, 0x5088},
	Rows:     8,
	Cols:     16,
	Keys:     crush80Keys,
	Custom: []string{
		"2.4G", "BT1", "BT2", "BT3", "Mode", "WinLk", "WinMac", "LogoM", "LogoC", "LogoP",
		"SideM", "SideC", "SideP", "Delay", "LEDP", "Batt",
	},
	Menu: &LightingMenu{Channel: 3, Brightness: 1, Effect: 2, Speed: 3, Color: 4},
	Effects: []string{
		"Off", "Wave", "Colour cloud", "Vortex", "Mix colour", "Breathe", "Light", "Slowly off", "Stone", "Laser",
		"Starry", "Flowers open", "Traverse", "Wave bar", "Meteor", "Rain", "Scan", "Trigger colour", "Center spread",
	},
	MaxBrightness: 9,
	MaxSpeed:      4,
	RawInterface:  1,
}

// crush80Keys is the ANSI board from Wobkey's VIA definition, without the
// alternative ISO and split keys that definition draws beside the board. The
// definition leaves out matrix 7,0 and 7,1, which hold the volume keys and
// appear to be the knob; they are drawn next to its press position at 4,12.
var crush80Keys = [][]Key{
	{
		k(0, 1, 0, 0, "Esc"),
		k(1.25, 1, 0, 1, "F1"), k(2.25, 1, 0, 2, "F2"), k(3.25, 1, 0, 3, "F3"), k(4.25, 1, 0, 4, "F4"),
		k(5.5, 1, 0, 5, "F5"), k(6.5, 1, 0, 6, "F6"), k(7.5, 1, 0, 7, "F7"), k(8.5, 1, 0, 8, "F8"),
		k(9.75, 1, 0, 9, "F9"), k(10.75, 1, 0, 10, "F10"), k(11.75, 1, 0, 11, "F11"), k(12.75, 1, 0, 12, "F12"),
		k(14, 1, 4, 12, "Knob"), k(15.25, 1, 7, 0, "KnobL"), k(16.25, 1, 7, 1, "KnobR"),
		k(17.75, 1, 0, 13, "PrtSc"), k(18.75, 1, 0, 14, "ScrLk"), k(19.75, 1, 0, 15, "Pause"),
	},
	{
		k(0, 1, 1, 0, "`"), k(1, 1, 1, 1, "1"), k(2, 1, 1, 2, "2"), k(3, 1, 1, 3, "3"), k(4, 1, 1, 4, "4"),
		k(5, 1, 1, 5, "5"), k(6, 1, 1, 6, "6"), k(7, 1, 1, 7, "7"), k(8, 1, 1, 8, "8"), k(9, 1, 1, 9, "9"),
		k(10, 1, 1, 10, "0"), k(11, 1, 1, 11, "-"), k(12, 1, 1, 12, "="), k(13, 2, 1, 13, "Bksp"),
		k(17.75, 1, 1, 14, "Ins"), k(18.75, 1, 1, 15, "Home"), k(19.75, 1, 3, 14, "PgUp"),
	},
	{
		k(0, 1.5, 2, 0, "Tab"), k(1.5, 1, 2, 1, "Q"), k(2.5, 1, 2, 2, "W"), k(3.5, 1, 2, 3, "E"), k(4.5, 1, 2, 4, "R"),
		k(5.5, 1, 2, 5, "T"), k(6.5, 1, 2, 6, "Y"), k(7.5, 1, 2, 7, "U"), k(8.5, 1, 2, 8, "I"), k(9.5, 1, 2, 9, "O"),
		k(10.5, 1, 2, 10, "P"), k(11.5, 1, 2, 11, "["), k(12.5, 1, 2, 12, "]"), k(13.5, 1.5, 2, 13, "\\"),
		k(17.75, 1, 2, 14, "Del"), k(18.75, 1, 2, 15, "End"), k(19.75, 1, 3, 15, "PgDn"),
	},
	{
		k(0, 1.75, 3, 0, "Caps"), k(1.75, 1, 3, 1, "A"), k(2.75, 1, 3, 2, "S"), k(3.75, 1, 3, 3, "D"), k(4.75, 1, 3, 4, "F"),
		k(5.75, 1, 3, 5, "G"), k(6.75, 1, 3, 6, "H"), k(7.75, 1, 3, 7, "J"), k(8.75, 1, 3, 8, "K"), k(9.75, 1, 3, 9, "L"),
		k(10.75, 1, 3, 10, ";"), k(11.75, 1, 3, 11, "'"), k(12.75, 2.25, 3, 13, "Enter"),
	},
	{
		k(0, 2.25, 4, 0, "LShft"), k(2.25, 1, 4, 2, "Z"), k(3.25, 1, 4, 3, "X"), k(4.25, 1, 4, 4, "C"), k(5.25, 1, 4, 5, "V"),
		k(6.25, 1, 4, 6, "B"), k(7.25, 1, 4, 7, "N"), k(8.25, 1, 4, 8, "M"), k(9.25, 1, 4, 9, ","), k(10.25, 1, 4, 10, "."),
		k(11.25, 1, 4, 11, "/"), k(12.25, 2.75, 4, 13, "RShft"),
		k(18.75, 1, 4, 14, "↑"),
	},
	{
		k(0, 1.25, 5, 0, "LCtrl"), k(1.25, 1.25, 5, 1, "LWin"), k(2.5, 1.25, 5, 2, "LAlt"),
		k(3.75, 6.25, 5, 5, "Space"),
		k(10, 1.25, 5, 9, "RAlt"), k(11.25, 1.25, 5, 10, "Fn"), k(12.5, 1.25, 5, 11, "RWin"), k(13.75, 1.25, 5, 12, "RCtrl"),
		k(17.75, 1, 5, 13, "←"), k(18.75, 1, 5, 14, "↓"), k(19.75, 1, 5, 15, "→"),
	},
}

func k(x, w float64, row, col int, stock string) Key {
	return Key{X: x, W: w, Row: row, Col: col, Stock: stock}
}
