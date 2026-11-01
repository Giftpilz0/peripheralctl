package via

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

const testRows, testCols = 8, 16

const (
	menuChannel    byte = 3
	menuBrightness byte = 1
	menuEffect     byte = 2
	menuSpeed      byte = 3
	menuColor      byte = 4
)

// fakeKeyboard implements the VIA commands in memory.
type fakeKeyboard struct {
	state       *State
	sent        [][]byte
	reply       []byte
	ignoreKeys  bool // accepts key writes but drops them
	saved       int
	resetOnMode bool // an effect change resets the brightness
}

func newFake() *fakeKeyboard {
	return &fakeKeyboard{state: &State{
		Protocol: 11, Layers: 2, Rows: testRows, Cols: testCols,
		Codes:    make([]uint16, 2*testRows*testCols),
		Lighting: Lighting{Brightness: 4, Effect: 6, Speed: 2, Hue: 20, Saturation: 255},
	}}
}

func (f *fakeKeyboard) Write(b []byte) (int, error) {
	f.sent = append(f.sent, append([]byte(nil), b...))
	req := b[1:]
	f.reply = make([]byte, reportSize)
	copy(f.reply, req)
	s := f.state
	l := &s.Lighting
	switch req[0] {
	case cmdProtocolVersion:
		f.reply[1], f.reply[2] = byte(s.Protocol>>8), byte(s.Protocol)
	case cmdLayerCount:
		f.reply[1] = byte(s.Layers)
	case cmdKeymapGet:
		off, size := int(req[1])<<8|int(req[2]), int(req[3])
		for i := range size {
			code := s.Codes[(off+i)/2]
			if (off+i)%2 == 0 {
				f.reply[4+i] = byte(code >> 8)
			} else {
				f.reply[4+i] = byte(code)
			}
		}
	case cmdSetKey:
		if !f.ignoreKeys {
			s.Set(int(req[1]), int(req[2]), int(req[3]), uint16(req[4])<<8|uint16(req[5]))
		}
	case cmdMenuGet:
		switch req[2] {
		case menuBrightness:
			f.reply[3] = l.Brightness
		case menuEffect:
			f.reply[3] = l.Effect
		case menuSpeed:
			f.reply[3] = l.Speed
		case menuColor:
			f.reply[3], f.reply[4] = l.Hue, l.Saturation
		}
	case cmdMenuSet:
		switch req[2] {
		case menuBrightness:
			l.Brightness = req[3]
		case menuEffect:
			l.Effect = req[3]
			if f.resetOnMode {
				l.Brightness = 0
			}
		case menuSpeed:
			l.Speed = req[3]
		case menuColor:
			l.Hue, l.Saturation = req[3], req[4]
		}
	case cmdMenuSave:
		f.saved++
	default:
		f.reply[0] = cmdUnhandled
	}
	return len(b), nil
}

func (f *fakeKeyboard) ReadReport(b []byte, _ time.Duration) (int, error) {
	if f.reply == nil {
		return 0, errors.New("nothing to read")
	}
	return copy(b, f.reply), nil
}

// writes returns the reports that change something.
func (f *fakeKeyboard) writes() [][]byte {
	var out [][]byte
	for _, r := range f.sent {
		switch r[1] {
		case cmdSetKey, cmdMenuSet, cmdMenuSave:
			out = append(out, r)
		}
	}
	return out
}

func deviceFor(f *fakeKeyboard) *Device {
	return &Device{p: f, close: func() error { return nil }, menu: Crush80.Menu, writable: true}
}

func read(t *testing.T, d *Device) *State {
	t.Helper()
	s, err := d.State(testRows, testCols)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStateRoundTrip(t *testing.T) {
	f := newFake()
	f.state.Set(1, 3, 5, 0x7f03)
	s := read(t, deviceFor(f))
	if !s.Equal(f.state) {
		t.Fatalf("read %+v, have %+v", s, f.state)
	}
	if got := s.At(1, 3, 5); got != 0x7f03 {
		t.Fatalf("At = %#x", got)
	}
	if len(f.writes()) != 0 {
		t.Fatal("reading sent a write")
	}
	for _, report := range f.sent {
		if len(report) != reportSize+1 || report[0] != 0 {
			t.Fatalf("malformed report % x", report)
		}
	}
}

// Wiping the keyboard, jumping to the bootloader, overwriting whole regions and
// touching macros must be refused before anything reaches it.
func TestOnlyAllowedCommandsAreSent(t *testing.T) {
	for _, cmd := range []byte{0x03, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x13, 0x15, 0x16} {
		if allowed(cmd) {
			t.Errorf("command %#02x must never be allowed", cmd)
		}
	}
	count := 0
	for cmd := range 256 {
		f := newFake()
		_, err := send(f, byte(cmd), 0, 0, 0)
		if allowed(byte(cmd)) {
			count++
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("command %#02x: err = %v", cmd, err)
		}
		if len(f.sent) != 0 {
			t.Errorf("command %#02x reached the keyboard", cmd)
		}
	}
	if count != 7 {
		t.Fatalf("%d commands allowed, want 7", count)
	}
}

func TestApplyEqualSendsNothing(t *testing.T) {
	f := newFake()
	d := deviceFor(f)
	cur := read(t, d)
	f.sent = nil
	c, err := d.Apply(cur, cur.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !c.None() || len(f.sent) != 0 {
		t.Fatalf("changes %v, %d reports sent", c, len(f.sent))
	}
}

func TestApplyWritesOnlyDifferences(t *testing.T) {
	f := newFake()
	d := deviceFor(f)
	cur := read(t, d)
	want := cur.Clone()
	want.Set(1, 2, 3, 0x0004)
	want.Set(0, 7, 15, 0x0005)
	f.sent = nil
	c, err := d.Apply(cur, want)
	if err != nil {
		t.Fatal(err)
	}
	if c.Keys != 2 || c.Lighting {
		t.Fatalf("changes %+v", c)
	}
	w := f.writes()
	if len(w) != 2 {
		t.Fatalf("%d writes", len(w))
	}
	if !bytes.Equal(w[0][1:7], []byte{cmdSetKey, 0, 7, 15, 0x00, 0x05}) {
		t.Fatalf("first write % x", w[0][:8])
	}
	if f.saved != 0 {
		t.Fatal("key changes must not save lighting")
	}
	if !f.state.Equal(want) {
		t.Fatal("keyboard does not match")
	}
}

func TestApplyLighting(t *testing.T) {
	f := newFake()
	f.resetOnMode = true
	d := deviceFor(f)
	cur := read(t, d)
	want := cur.Clone()
	want.Lighting = Lighting{Brightness: 4, Effect: 1, Speed: 3, Hue: 100, Saturation: 200}
	c, err := d.Apply(cur, want)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Lighting || f.saved != 1 {
		t.Fatalf("changes %+v, saved %d", c, f.saved)
	}
	if f.state.Lighting != want.Lighting {
		t.Fatalf("lighting %+v", f.state.Lighting)
	}
}

func TestApplyDetectsIgnoredWrite(t *testing.T) {
	f := newFake()
	f.ignoreKeys = true
	d := deviceFor(f)
	cur := read(t, d)
	want := cur.Clone()
	want.Set(0, 0, 0, 0x0004)
	if _, err := d.Apply(cur, want); err == nil || !strings.Contains(err.Error(), "verify") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyRefusesIncompatible(t *testing.T) {
	f := newFake()
	d := deviceFor(f)
	cur := read(t, d)
	for name, change := range map[string]func(*State){
		"protocol": func(s *State) { s.Protocol = 12 },
		"layers":   func(s *State) { s.Layers = 3 },
		"codes":    func(s *State) { s.Codes = s.Codes[:5] },
	} {
		want := cur.Clone()
		change(want)
		f.sent = nil
		if _, err := d.Apply(cur, want); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if len(f.sent) != 0 {
			t.Errorf("%s: sent %d reports", name, len(f.sent))
		}
	}
}

func TestSendSkipsForeignReports(t *testing.T) {
	f := &scriptedPort{replies: [][]byte{{0x55, 1, 2}, {cmdLayerCount, 4}}}
	r, err := send(f, cmdLayerCount)
	if err != nil {
		t.Fatal(err)
	}
	if r[1] != 4 {
		t.Fatalf("reply %v", r)
	}
}

func TestSendUnhandled(t *testing.T) {
	f := &scriptedPort{replies: [][]byte{{cmdUnhandled}}}
	if _, err := send(f, cmdLayerCount); err == nil {
		t.Fatal("expected error")
	}
}

func TestSendTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the timeout")
	}
	f := &scriptedPort{}
	if _, err := send(f, cmdLayerCount); err == nil || !strings.Contains(err.Error(), "no reply") {
		t.Fatalf("err = %v", err)
	}
}

type scriptedPort struct{ replies [][]byte }

func (s *scriptedPort) Write(b []byte) (int, error) { return len(b), nil }

func (s *scriptedPort) ReadReport(b []byte, timeout time.Duration) (int, error) {
	if len(s.replies) == 0 {
		time.Sleep(timeout)
		return 0, nil
	}
	n := copy(b, s.replies[0])
	s.replies = s.replies[1:]
	return n, nil
}

func TestLabel(t *testing.T) {
	custom := []string{"2.4G", "BT1"}
	tests := []struct {
		protocol int
		code     uint16
		want     string
	}{
		{12, 0x0000, ""},
		{12, 0x0001, "▽"},
		{12, 0x0004, "A"},
		{12, 0x001e, "1"},
		{12, 0x0027, "0"},
		{12, 0x003a, "F1"},
		{12, 0x0073, "F24"},
		{12, 0x00e0, "LCtrl"},
		{12, 0x0104, "C+A"},
		{12, 0x1204, "RS+A"},
		{12, 0x4105, "LT1 B"},
		{12, 0x5221, "MO1"},
		{12, 0x5201, "TO1"},
		{12, 0x5262, "TG2"},
		{12, 0x2204, "S/A"},
		{12, 0x7e01, "BT1"},
		{12, 0x7e09, "Custom9"},
		{12, 0x7700, "M0"},
		{12, 0x7820, "RGB"},
		{12, 0x7c00, "Boot"},
		{12, 0x00cd, "MsUp"},
		{12, 0x5fff, "0x5fff"},
		{12, 0xffff, "fw"},
		{11, 0x7f00, "2.4G"},
		{11, 0x7702, "M0"},
		{11, 0x5221, "MO1"},
		{10, 0x5101, "MO1"},
		{10, 0x5f80, "2.4G"},
		{10, 0x5f12, "M0"},
		{10, 0x5cc2, "RGB"},
		{10, 0x6204, "S/A"},
		{10, 0x00f0, "MsUp"},
		{10, 0x5001, "TO1"},
		{9, 0x5011, "TO1"},
		{9, 0x5c00, "Boot"},
		{13, 0x5221, "0x5221"}, // unknown table: hex rather than a wrong name
		{13, 0x0104, "C+A"},
	}
	for _, tt := range tests {
		if got := Label(tt.protocol, tt.code, custom); got != tt.want {
			t.Errorf("Label(%d, %#04x) = %q, want %q", tt.protocol, tt.code, got, tt.want)
		}
	}
}

func TestCatalogRoundTrips(t *testing.T) {
	custom := []string{"2.4G", "BT1"}
	for _, protocol := range []int{9, 10, 11, 12} {
		entries := Catalog(protocol, 4, custom)
		if len(entries) < 150 {
			t.Fatalf("protocol %d: %d entries", protocol, len(entries))
		}
		seen := map[uint16]bool{}
		for _, e := range entries {
			if seen[e.Code] {
				t.Errorf("protocol %d: %#04x listed twice", protocol, e.Code)
			}
			seen[e.Code] = true
			if Dangerous(protocol, e.Code) {
				t.Errorf("protocol %d: dangerous %#04x offered", protocol, e.Code)
			}
			if e.Name == "" || strings.HasPrefix(e.Name, "0x") {
				t.Errorf("protocol %d: %#04x has name %q", protocol, e.Code, e.Name)
			}
		}
	}
}

func TestDangerous(t *testing.T) {
	for _, tt := range []struct {
		protocol int
		code     uint16
		want     bool
	}{
		{11, 0x7c00, true},
		{11, 0x7c03, true},
		{10, 0x5c00, true},
		{10, 0x0000, false},
		{13, 0x7c00, true}, // unknown table: anything beyond plain keys is refused
		{13, 0x0104, false},
		{11, 0x0004, false},
		{11, 0x5221, false},
	} {
		if got := Dangerous(tt.protocol, tt.code); got != tt.want {
			t.Errorf("Dangerous(%d, %#04x) = %v", tt.protocol, tt.code, got)
		}
	}
}

func TestReadOnlyDeviceRefusesWrites(t *testing.T) {
	f := newFake()
	d := deviceFor(f)
	d.writable = false
	cur := read(t, d)
	want := cur.Clone()
	want.Set(0, 0, 0, 0x0004)
	want.Lighting.Hue = 99
	if _, err := d.Apply(cur, want); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("apply: %v", err)
	}
	if err := d.PreviewColor(Color{99, 255}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("preview: %v", err)
	}
	if got := f.writes(); len(got) != 0 {
		t.Fatalf("reached the keyboard: %v", got)
	}
}

func TestStateWithoutLightingMenu(t *testing.T) {
	f := newFake()
	d := deviceFor(f)
	d.menu = nil
	if s := read(t, d); s.Lighting != (Lighting{}) {
		t.Fatalf("lighting %+v", s.Lighting)
	}
	for _, r := range f.sent {
		if r[1] == cmdMenuGet {
			t.Fatal("asked for lighting a keyboard does not have")
		}
	}
}

func TestLookupOnlyTrustsTheWhitelist(t *testing.T) {
	for _, product := range Crush80.Products {
		if d := Lookup(Crush80.Vendor, product, nil); d == nil || !d.WriteSafe {
			t.Errorf("product %#04x: %+v", product, d)
		}
	}
	def, err := ParseDefinition([]byte(testDefinition))
	if err != nil {
		t.Fatal(err)
	}
	if def.WriteSafe {
		t.Fatal("a definition file made itself writable")
	}
	got := Lookup(def.Vendor, def.Products[0], []*Definition{def})
	if got == nil || got.WriteSafe {
		t.Fatalf("imported definition: %+v", got)
	}
	if Lookup(0x1234, 0x0001, nil) != nil {
		t.Fatal("an unknown keyboard matched")
	}
}

// The JSON tries to claim write access; only the code's whitelist can grant it.
const testDefinition = `{
	"name": "Test Pad", "vendorId": "0xFEED", "productId": "0x0042", "writeSafe": true, "WriteSafe": true,
	"matrix": {"rows": 2, "cols": 3},
	"keycodes": ["qmk_lighting"],
	"customKeycodes": [{"name": "A thing", "title": "x", "shortName": "THING"}, {"name": "No short"}],
	"menus": [{"label": "Lighting", "content": [{"label": "Backlight", "content": [
		{"label": "Brightness", "type": "range", "options": [0, 200], "content": ["id_qmk_rgb_matrix_brightness", 3, 1]},
		{"label": "Effect", "type": "dropdown", "options": [["Off", 0], ["Solid", 1]], "content": ["id_qmk_rgb_matrix_effect", 3, 2]},
		{"label": "Speed", "type": "range", "options": [0, 7], "content": ["id_qmk_rgb_matrix_effect_speed", 3, 3]},
		{"label": "Color", "type": "color", "content": ["id_qmk_rgb_matrix_color", 3, 4]}
	]}]}],
	"layouts": {"labels": ["ISO"], "keymap": [
		[{"c": "#aaa"}, "0,0", {"x": 0.5, "w": 2}, "0,1", "0,2\n\n\n0,0", "1,2\n\n\n0,1"],
		[{"y": 0.25, "d": true}, "x", "1,0", {"w": 1.5}, "1,1"]
	]}
}`

func TestParseDefinition(t *testing.T) {
	d, err := ParseDefinition([]byte(testDefinition))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Test Pad" || d.Vendor != 0xfeed || d.Products[0] != 0x0042 || d.Rows != 2 || d.Cols != 3 {
		t.Fatalf("%+v", d)
	}
	if len(d.Custom) != 2 || d.Custom[0] != "THING" || d.Custom[1] != "No short" {
		t.Fatalf("custom %v", d.Custom)
	}
	if d.Menu == nil || *d.Menu != (LightingMenu{3, 1, 2, 3, 4}) || d.MaxBrightness != 200 || d.MaxSpeed != 7 {
		t.Fatalf("menu %+v, max %d, %d", d.Menu, d.MaxBrightness, d.MaxSpeed)
	}
	if len(d.Effects) != 2 || d.Effects[1] != "Solid" {
		t.Fatalf("effects %v", d.Effects)
	}
	if len(d.Keys) != 2 || len(d.Keys[0]) != 3 || len(d.Keys[1]) != 2 {
		t.Fatalf("keys %+v", d.Keys)
	}
	if k := d.Keys[0][1]; k.Row != 0 || k.Col != 1 || k.X != 1.5 || k.W != 2 {
		t.Fatalf("second key %+v", k)
	}
	if k := d.Keys[0][2]; k.Col != 2 || k.Row != 0 {
		t.Fatalf("layout option not filtered: %+v", k)
	}
	if k := d.Keys[1][1]; k.W != 1.5 || k.Stock != "1,1" {
		t.Fatalf("width %+v", k)
	}
}

// Wobkey's own file lists unmarked alternatives like these.
func TestParseDefinitionDropsUnmarkedAlternatives(t *testing.T) {
	d, err := ParseDefinition([]byte(`{"name":"x","vendorId":"0x1","productId":"0x2","matrix":{"rows":2,"cols":4},"layouts":{"keymap":[
		[{"x":-2.5},"1,0","1,1",{"x":0.5},"0,0","0,1",{"x":-1},"0,2",{"x":2},"0,0"],
		["1,2"]
	]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range d.Keys {
		for _, k := range row {
			got = append(got, k.Stock)
		}
	}
	if want := "0,0 0,1 1,2"; strings.Join(got, " ") != want {
		t.Fatalf("keys %v, want %s", got, want)
	}
}

func TestParseDefinitionRejects(t *testing.T) {
	for name, text := range map[string]string{
		"not JSON":   `{`,
		"no matrix":  `{"name":"x","vendorId":"0x1","productId":"0x2","layouts":{"keymap":[["0,0"]]}}`,
		"no vendor":  `{"name":"x","matrix":{"rows":1,"cols":1},"layouts":{"keymap":[["0,0"]]}}`,
		"no keys":    `{"name":"x","vendorId":"0x1","productId":"0x2","matrix":{"rows":1,"cols":1},"layouts":{"keymap":[]}}`,
		"off matrix": `{"name":"x","vendorId":"0x1","productId":"0x2","matrix":{"rows":1,"cols":1},"layouts":{"keymap":[["5,5"]]}}`,
		"rotated":    `{"name":"x","vendorId":"0x1","productId":"0x2","matrix":{"rows":1,"cols":1},"layouts":{"keymap":[[{"r":15},"0,0"]]}}`,
	} {
		if _, err := ParseDefinition([]byte(text)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestCrush80IsConsistent(t *testing.T) {
	total := 0
	seen := map[[2]int]string{}
	for _, row := range Crush80.Keys {
		end := 0.0
		for _, k := range row {
			total++
			if k.X < end {
				t.Errorf("%s overlaps the key before it", k.Stock)
			}
			end = k.X + k.W
			if k.Row >= Crush80.Rows || k.Col >= Crush80.Cols {
				t.Errorf("%s is outside the matrix", k.Stock)
			}
			pos := [2]int{k.Row, k.Col}
			if other, dup := seen[pos]; dup {
				t.Errorf("%s and %s share matrix %v", k.Stock, other, pos)
			}
			seen[pos] = k.Stock
		}
	}
	if total != 90 {
		t.Errorf("%d keys, want 90", total)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Crush 80": "crush-80", "  Foo/Bar_v2!! ": "foo-bar-v2", "..": "keyboard"} {
		if got := (&Definition{Name: in}).Slug(); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
