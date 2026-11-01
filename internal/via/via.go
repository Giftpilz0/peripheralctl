// Package via reads and changes the settings of a VIA keyboard over raw HID.
//
// send only passes the commands listed in allowed. Commands that wipe the
// keyboard, jump to the bootloader or overwrite whole regions are not defined
// here, so they cannot be sent. Apply writes only what differs and verifies the
// result by reading it back.
//
// Writes also need a model on the whitelist in definition.go; every other
// keyboard is read-only.
//
// Macros are left out on purpose: the Crush 80 acknowledges macro writes but
// reads its macro buffer as zeros, so they can be neither verified nor restored.
package via

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/unix"

	"github.com/giftpilz0/peripheralctl/internal/hidraw"
)

const (
	cmdProtocolVersion byte = 0x01
	cmdSetKey          byte = 0x05
	cmdMenuSet         byte = 0x07
	cmdMenuGet         byte = 0x08
	cmdMenuSave        byte = 0x09
	cmdLayerCount      byte = 0x11
	cmdKeymapGet       byte = 0x12

	cmdUnhandled byte = 0xff // the firmware's reply to an unknown command

	reportSize = 32
	maxChunk   = reportSize - 4 // command, 16-bit offset and size come first
	timeout    = time.Second
	maxLayers  = 32
)

// rawHIDUsage starts the report descriptor of QMK's raw HID interface: usage
// page 0xFF60, usage 0x61. The keyboard's other interfaces carry keystrokes.
var rawHIDUsage = []byte{0x06, 0x60, 0xff, 0x09, 0x61}

// ErrReadOnly means a write was refused because the model is not on the whitelist.
var ErrReadOnly = errors.New("read-only: this keyboard is not on the write whitelist")

// ErrNoAccess means the raw HID node exists but may not be opened.
var ErrNoAccess = errors.New("no access to the keyboard")

type port interface {
	Write(b []byte) (int, error)
	// ReadReport fills b or returns 0 bytes when nothing arrived in time.
	ReadReport(b []byte, timeout time.Duration) (int, error)
}

func allowed(cmd byte) bool {
	switch cmd {
	case cmdProtocolVersion, cmdMenuGet, cmdLayerCount, cmdKeymapGet, cmdSetKey, cmdMenuSet, cmdMenuSave:
		return true
	}
	return false
}

// send issues one command and returns the reply, which starts with the echo
// of cmd and args.
func send(p port, cmd byte, args ...byte) ([]byte, error) {
	if !allowed(cmd) {
		return nil, fmt.Errorf("via: command %#02x is not allowed", cmd)
	}
	if len(args) > reportSize-1 {
		return nil, fmt.Errorf("via: command %#02x: %d argument bytes", cmd, len(args))
	}
	// Report ID 0 (the interface has no numbered reports), then the payload.
	out := make([]byte, reportSize+1)
	out[1] = cmd
	copy(out[2:], args)
	if _, err := p.Write(out); err != nil {
		return nil, fmt.Errorf("via: send: %w", err)
	}
	echo := append([]byte{cmd}, args...)
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, fmt.Errorf("via: no reply to command %#02x", cmd)
		}
		in := make([]byte, reportSize)
		n, err := p.ReadReport(in, left)
		if err != nil {
			return nil, fmt.Errorf("via: receive: %w", err)
		}
		if n > 0 && in[0] == cmdUnhandled {
			return nil, fmt.Errorf("via: firmware does not support command %#02x", cmd)
		}
		if n >= len(echo) && bytes.Equal(in[:len(echo)], echo) {
			return in[:n], nil
		}
	}
}

// Lighting is the backlight state, as the keyboard reports it.
type Lighting struct {
	Brightness, Effect, Speed, Hue, Saturation byte
}

// Color is the backlight colour as the keyboard reports it.
type Color struct{ Hue, Saturation byte }

// Color returns the stored colour.
func (l Lighting) Color() Color { return Color{l.Hue, l.Saturation} }

// State is everything this package can read and write on the keyboard.
type State struct {
	Protocol, Layers, Rows, Cols int
	Codes                        []uint16 // layer, then row, then column
	Lighting                     Lighting
}

func (s *State) index(layer, row, col int) int { return (layer*s.Rows+row)*s.Cols + col }

// At returns the keycode at a matrix position.
func (s *State) At(layer, row, col int) uint16 { return s.Codes[s.index(layer, row, col)] }

// Set changes the keycode at a matrix position in this copy only.
func (s *State) Set(layer, row, col int, code uint16) { s.Codes[s.index(layer, row, col)] = code }

// Clone returns an independent copy.
func (s *State) Clone() *State {
	c := *s
	c.Codes = slices.Clone(s.Codes)
	return &c
}

// Equal tells whether both describe the same keyboard contents.
func (s *State) Equal(o *State) bool {
	return s.Protocol == o.Protocol && s.Layers == o.Layers && s.Rows == o.Rows && s.Cols == o.Cols &&
		s.Lighting == o.Lighting && slices.Equal(s.Codes, o.Codes)
}

// Compatible reports why want cannot be written over s, if it cannot.
// Keycode numbers differ between protocol versions.
func (s *State) Compatible(want *State) error {
	switch {
	case s.Protocol != want.Protocol:
		return fmt.Errorf("protocol %d on the keyboard, %d in the profile", s.Protocol, want.Protocol)
	case s.Layers != want.Layers || s.Rows != want.Rows || s.Cols != want.Cols:
		return fmt.Errorf("keyboard has %d layers of %dx%d, profile %d of %dx%d",
			s.Layers, s.Rows, s.Cols, want.Layers, want.Rows, want.Cols)
	case len(want.Codes) != want.Layers*want.Rows*want.Cols:
		return fmt.Errorf("profile has %d keycodes", len(want.Codes))
	}
	return nil
}

// Changes counts the differences between two states.
type Changes struct {
	Keys     int
	Lighting bool
}

// None tells whether there is nothing to write.
func (c Changes) None() bool { return c.Keys == 0 && !c.Lighting }

func (c Changes) String() string {
	if c.None() {
		return "no differences"
	}
	s := fmt.Sprintf("%d keys", c.Keys)
	if c.Lighting {
		s += ", lighting"
	}
	return s
}

// Diff counts what writing want over s would change.
func (s *State) Diff(want *State) Changes {
	c := Changes{Lighting: s.Lighting != want.Lighting}
	for i := range min(len(s.Codes), len(want.Codes)) {
		if s.Codes[i] != want.Codes[i] {
			c.Keys++
		}
	}
	return c
}

// Device is an open raw HID node.
type Device struct {
	p        port
	close    func() error
	menu     *LightingMenu // nil when the keyboard has no lighting controls
	writable bool
}

// Open opens the raw HID node of a keyboard described by def.
func Open(path string, def *Definition) (*Device, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			return nil, fmt.Errorf("%w: %s", ErrNoAccess, path)
		}
		return nil, fmt.Errorf("via: open %s: %w", path, err)
	}
	return &Device{p: fdPort(fd), close: func() error { return unix.Close(fd) }, menu: def.Menu, writable: def.WriteSafe}, nil
}

// Close releases the node.
func (d *Device) Close() error { return d.close() }

func (d *Device) readLighting() (Lighting, error) {
	var l Lighting
	if d.menu == nil {
		return l, nil
	}
	for _, f := range []struct {
		id  byte
		dst []*byte
	}{
		{d.menu.Brightness, []*byte{&l.Brightness}},
		{d.menu.Effect, []*byte{&l.Effect}},
		{d.menu.Speed, []*byte{&l.Speed}},
		{d.menu.Color, []*byte{&l.Hue, &l.Saturation}},
	} {
		if f.id == 0 {
			continue
		}
		r, err := send(d.p, cmdMenuGet, d.menu.Channel, f.id)
		if err != nil {
			return l, err
		}
		if len(r) < 3+len(f.dst) {
			return l, errors.New("via: short lighting reply")
		}
		for i, dst := range f.dst {
			*dst = r[3+i]
		}
	}
	return l, nil
}

// State reads everything VIA exposes; it only sends queries.
func (d *Device) State(rows, cols int) (*State, error) {
	r, err := send(d.p, cmdProtocolVersion)
	if err != nil {
		return nil, err
	}
	if len(r) < 3 {
		return nil, errors.New("via: short protocol version reply")
	}
	s := &State{Protocol: int(r[1])<<8 | int(r[2]), Rows: rows, Cols: cols}

	if r, err = send(d.p, cmdLayerCount); err != nil {
		return nil, err
	}
	if len(r) < 2 || r[1] == 0 || r[1] > maxLayers {
		return nil, fmt.Errorf("via: implausible layer count reply %v", r)
	}
	s.Layers = int(r[1])

	total := s.Layers * rows * cols * 2
	raw := make([]byte, 0, total)
	for len(raw) < total {
		off := len(raw)
		size := min(maxChunk, total-off)
		r, err := send(d.p, cmdKeymapGet, byte(off>>8), byte(off), byte(size))
		if err != nil {
			return nil, err
		}
		if len(r) < 4+size {
			return nil, errors.New("via: short keymap reply")
		}
		raw = append(raw, r[4:4+size]...)
	}
	s.Codes = make([]uint16, len(raw)/2)
	for i := range s.Codes {
		s.Codes[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}

	if s.Lighting, err = d.readLighting(); err != nil {
		return nil, err
	}
	return s, nil
}

func (d *Device) writeLighting(cur, want Lighting) error {
	set := func(id byte, v ...byte) error {
		_, err := send(d.p, cmdMenuSet, append([]byte{d.menu.Channel, id}, v...)...)
		return err
	}
	// Changing the effect may reset the others, so they are compared after it.
	if cur.Effect != want.Effect {
		if err := set(d.menu.Effect, want.Effect); err != nil {
			return err
		}
		var err error
		if cur, err = d.readLighting(); err != nil {
			return err
		}
	}
	if cur.Brightness != want.Brightness {
		if err := set(d.menu.Brightness, want.Brightness); err != nil {
			return err
		}
	}
	if cur.Speed != want.Speed {
		if err := set(d.menu.Speed, want.Speed); err != nil {
			return err
		}
	}
	if cur.Hue != want.Hue || cur.Saturation != want.Saturation {
		if err := set(d.menu.Color, want.Hue, want.Saturation); err != nil {
			return err
		}
	}
	return nil
}

// PreviewColor changes the backlight colour without the save command.
func (d *Device) PreviewColor(c Color) error {
	if !d.writable {
		return ErrReadOnly
	}
	if d.menu == nil {
		return errors.New("via: this keyboard has no lighting controls")
	}
	_, err := send(d.p, cmdMenuSet, d.menu.Channel, d.menu.Color, c.Hue, c.Saturation)
	return err
}

// Apply writes the differences between cur, which must be freshly read, and
// want, then reads the keyboard back. It sends nothing when they are equal.
func (d *Device) Apply(cur, want *State) (Changes, error) {
	if !d.writable {
		return Changes{}, ErrReadOnly
	}
	if err := cur.Compatible(want); err != nil {
		return Changes{}, err
	}
	c := cur.Diff(want)
	if c.None() {
		return c, nil
	}
	if c.Keys > 0 {
		for layer := range want.Layers {
			for row := range want.Rows {
				for col := range want.Cols {
					code := want.At(layer, row, col)
					if code == cur.At(layer, row, col) {
						continue
					}
					if _, err := send(d.p, cmdSetKey, byte(layer), byte(row), byte(col), byte(code>>8), byte(code)); err != nil {
						return c, err
					}
				}
			}
		}
	}
	if c.Lighting && d.menu != nil {
		if err := d.writeLighting(cur.Lighting, want.Lighting); err != nil {
			return c, err
		}
		if _, err := send(d.p, cmdMenuSave, d.menu.Channel); err != nil {
			return c, err
		}
	}
	got, err := d.State(want.Rows, want.Cols)
	if err != nil {
		return c, fmt.Errorf("via: verify: %w", err)
	}
	if !got.Equal(want) {
		return c, fmt.Errorf("via: verify: the keyboard differs after writing: %s", got.Diff(want))
	}
	return c, nil
}

// Detected is a keyboard exposing VIA's raw HID interface.
type Detected struct {
	Path            string
	Vendor, Product uint16
	Interface       int // USB interface number, -1 if unknown
}

// Detect lists VIA keyboards from sysfs, without opening them.
func Detect() ([]Detected, error) {
	nodes, err := hidraw.All()
	if err != nil {
		return nil, err
	}
	var out []Detected
	for _, n := range nodes {
		desc, err := os.ReadFile(filepath.Join(n.SysPath, "report_descriptor"))
		if err == nil && bytes.HasPrefix(desc, rawHIDUsage) {
			out = append(out, Detected{n.Path, n.Vendor, n.Product, n.Interface})
		}
	}
	return out, nil
}

type fdPort int

func (fd fdPort) Write(b []byte) (int, error) { return unix.Write(int(fd), b) }

func (fd fdPort) ReadReport(b []byte, timeout time.Duration) (int, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, int(timeout.Milliseconds())+1)
	if err != nil {
		if errors.Is(err, unix.EINTR) {
			return 0, nil
		}
		return 0, err
	}
	if n == 0 {
		return 0, nil // send notices the passed deadline
	}
	return unix.Read(int(fd), b)
}
