package kbdtui

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/unix"

	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// The key tester shows what the computer receives from the keyboard. It reads
// the input event nodes only while it runs, never grabs them and keeps nothing
// but the keys on screen.

const (
	evKey   = 1
	evMsc   = 4
	mscScan = 4
)

var inputEventSize = int(unsafe.Sizeof(unix.Timeval{})) + 8

type inputEvent struct {
	scan uint32 // HID usage: page in the upper 16 bits
	code uint16 // Linux key code
	down bool
}

type tester struct {
	events  chan inputEvent
	stop    chan struct{}
	pressed map[uint16]bool // keycodes held down
	last    string
	count   int
}

type testerMsg struct {
	t  *tester
	ev inputEvent
	ok bool
}

// inputNodes lists the event devices of a keyboard, read from sysfs below root.
func inputNodes(root string, def *via.Definition) []string {
	dirs, _ := filepath.Glob(filepath.Join(root, "event*"))
	var out []string
	for _, dir := range dirs {
		read := func(name string) uint64 {
			data, err := os.ReadFile(filepath.Join(dir, "device", "id", name))
			if err != nil {
				return 0
			}
			v, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 16, 16)
			return v
		}
		if def.Matches(uint16(read("vendor")), uint16(read("product"))) {
			out = append(out, filepath.Join("/dev/input", filepath.Base(dir)))
		}
	}
	return out
}

func (m *model) startTester() tea.Cmd {
	nodes := inputNodes("/sys/class/input", m.def)
	if len(nodes) == 0 {
		m.err = errors.New("key test: the keyboard is not connected")
		return nil
	}
	t := &tester{events: make(chan inputEvent), stop: make(chan struct{}), pressed: map[uint16]bool{}}
	opened := 0
	for _, path := range nodes {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		opened++
		go t.read(fd)
	}
	if opened == 0 {
		m.err = errors.New("key test: no read access to the keyboard's input events; join the input group: sudo usermod -aG input $USER, then log in again")
		return nil
	}
	m.tester = t
	return t.next()
}

func (m *model) stopTester() {
	if m.tester != nil {
		close(m.tester.stop)
		m.tester = nil
	}
}

func (t *tester) next() tea.Cmd {
	return func() tea.Msg {
		select {
		case ev := <-t.events:
			return testerMsg{t, ev, true}
		case <-t.stop:
			return testerMsg{t: t}
		}
	}
}

// read forwards key events from one node until the tester stops.
func (t *tester) read(fd int) {
	defer func() { _ = unix.Close(fd) }()
	buf := make([]byte, inputEventSize*32)
	var scan uint32
	for {
		select {
		case <-t.stop:
			return
		default:
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 100)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return
		}
		if n <= 0 {
			continue
		}
		size, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			return // unplugged
		}
		for off := 0; off+inputEventSize <= size; off += inputEventSize {
			ev, ok := parseEvent(buf[off:off+inputEventSize], &scan)
			if !ok {
				continue
			}
			select {
			case t.events <- ev:
			case <-t.stop:
				return
			}
		}
	}
}

// parseEvent turns one input_event into a key press or release. The scan event
// comes just before its key event, so it is remembered in scan.
func parseEvent(b []byte, scan *uint32) (inputEvent, bool) {
	off := len(b) - 8
	typ := binary.NativeEndian.Uint16(b[off:])
	code := binary.NativeEndian.Uint16(b[off+2:])
	value := int32(binary.NativeEndian.Uint32(b[off+4:]))
	switch {
	case typ == evMsc && code == mscScan:
		*scan = uint32(value)
	case typ == evKey && value != 2: // 2 is auto-repeat
		ev := inputEvent{scan: *scan, code: code, down: value == 1}
		*scan = 0
		return ev, true
	}
	return inputEvent{}, false
}

// usageCode maps a HID usage to the VIA keycode that sends it.
func usageCode(scan uint32) (uint16, bool) {
	switch scan >> 16 {
	case 0x07:
		return uint16(scan & 0xff), true
	case 0x0c:
		code, ok := map[uint32]uint16{
			0xe2: 0xa8, 0xe9: 0xa9, 0xea: 0xaa, 0xb5: 0xab, 0xb6: 0xac, 0xb7: 0xad, 0xcd: 0xae,
		}[scan&0xffff]
		return code, ok
	}
	return 0, false
}

func (m *model) handleTester(msg testerMsg) tea.Cmd {
	t := msg.t
	if !msg.ok || t != m.tester {
		return nil
	}
	code, known := usageCode(msg.ev.scan)
	if known {
		t.pressed[code] = msg.ev.down
	}
	if msg.ev.down {
		t.count++
		switch {
		case !known:
			t.last = fmt.Sprintf("usage %#x, Linux key %d: not matched to a keycode", msg.ev.scan, msg.ev.code)
		case m.state == nil:
			t.last = fmt.Sprintf("keycode %#04x", code)
		default:
			t.last = via.Label(m.state.Protocol, code, m.def.Custom)
			if !m.onLayer(code) {
				t.last += fmt.Sprintf(" (no key sends this on layer %d; switch layers before the test)", m.layer)
			}
		}
	}
	if t.events == nil {
		return nil // only in tests
	}
	return t.next()
}

func (m *model) onLayer(code uint16) bool {
	for _, row := range m.def.Keys {
		for _, k := range row {
			if m.code(k) == code {
				return true
			}
		}
	}
	return false
}

func (t *tester) view() string {
	if t.count == 0 {
		return theme.Title.Render("Key test") + theme.Dim.Render("  press a key; matching keys on this layer light up")
	}
	return theme.Title.Render("Key test") + "  computer received: " + t.last
}
