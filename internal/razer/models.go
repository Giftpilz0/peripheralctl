package razer

import "time"

// Model describes one USB product of a mouse. The commands are the same for all
// of them; transaction IDs, reply delays and features differ (per OpenRazer).
//
// WriteSafe is the whitelist: only models tested with this program get writes,
// everything else is read-only. Add a model with WriteSafe false, test it, and
// only then set it. A wrong transaction ID makes the mouse ignore a request.
type Model struct {
	Name      string
	PID       uint16
	Wired     bool
	TID       byte // firmware, serial, battery, DPI, polling rate
	LightTID  byte // lighting
	MiscTID   byte // DPI stages, sleep timer, low-battery warning
	Wait      time.Duration
	Battery   bool
	Stages    bool
	Power     bool // sleep timer and low-battery warning
	WriteSafe bool
}

const (
	waitShort    = time.Millisecond // OpenRazer waits 600 µs; Sleep rounds up anyway
	waitReceiver = 31 * time.Millisecond
	waitViper    = 60 * time.Millisecond
)

// Models lists wired products before their receivers, so a scan prefers the cable.
var Models = []Model{
	{Name: "DeathAdder V2 Pro", PID: 0x007c, Wired: true, TID: 0x3f, LightTID: 0x3f, MiscTID: 0x3f, Wait: waitViper, Battery: true, Stages: true, Power: true, WriteSafe: true},
	{Name: "DeathAdder V2 Pro", PID: 0x007d, TID: 0x3f, LightTID: 0x3f, MiscTID: 0x3f, Wait: waitViper, Battery: true, Stages: true, Power: true, WriteSafe: true},

	{Name: "DeathAdder V2", PID: 0x0084, Wired: true, TID: 0x3f, LightTID: 0x3f, Wait: waitShort},
	{Name: "DeathAdder V2 Mini", PID: 0x008c, Wired: true, TID: 0x3f, LightTID: 0x3f, MiscTID: 0xff, Wait: waitShort, Stages: true},
	{Name: "DeathAdder Elite", PID: 0x005c, Wired: true, TID: 0x3f, LightTID: 0x3f, Wait: waitShort},
	{Name: "Basilisk", PID: 0x0064, Wired: true, TID: 0x3f, LightTID: 0x3f, Wait: waitShort},
	{Name: "Basilisk Essential", PID: 0x0065, Wired: true, TID: 0x3f, LightTID: 0x3f, MiscTID: 0xff, Wait: waitShort, Stages: true},
	{Name: "Mamba Wireless", PID: 0x0073, Wired: true, TID: 0x3f, LightTID: 0x3f, MiscTID: 0xff, Wait: waitReceiver, Battery: true, Power: true},
	{Name: "Mamba Wireless", PID: 0x0072, TID: 0x3f, LightTID: 0x3f, MiscTID: 0xff, Wait: waitReceiver, Battery: true, Power: true},
	{Name: "Viper", PID: 0x0078, Wired: true, TID: 0xff, LightTID: 0x3f, MiscTID: 0xff, Wait: waitShort, Stages: true},
	{Name: "Viper Mini", PID: 0x008a, Wired: true, TID: 0xff, LightTID: 0x3f, MiscTID: 0xff, Wait: waitShort, Stages: true},
	{Name: "Viper Ultimate", PID: 0x007a, Wired: true, TID: 0xff, LightTID: 0x3f, MiscTID: 0xff, Wait: waitViper, Battery: true, Stages: true, Power: true},
	{Name: "Viper Ultimate", PID: 0x007b, TID: 0xff, LightTID: 0x3f, MiscTID: 0xff, Wait: waitViper, Battery: true, Stages: true, Power: true},
}

func modelFor(pid uint16) (Model, bool) {
	for _, m := range Models {
		if m.PID == pid {
			return m, true
		}
	}
	return Model{}, false
}

// ProductIDs lists every Razer product this program may talk to.
func ProductIDs() []uint16 {
	ids := []uint16{PIDDock}
	for _, m := range Models {
		ids = append(ids, m.PID)
	}
	return ids
}
