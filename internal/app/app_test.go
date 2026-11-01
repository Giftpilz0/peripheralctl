package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/giftpilz0/peripheralctl/internal/kbdtui"
)

type fakeScreen struct {
	keys, others int
	modal        bool
	closed       int
	pending      bool
	armed        bool
	rescanBusy   bool
	rescans      int
	found        []kbdtui.Found
}

func (f *fakeScreen) Init() tea.Cmd { return nil }
func (f *fakeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		f.keys++
	} else {
		f.others++
	}
	return f, nil
}
func (f *fakeScreen) View() string { return "" }
func (f *fakeScreen) Modal() bool  { return f.modal }
func (f *fakeScreen) Close()       { f.closed++ }
func (f *fakeScreen) Rescan() (tea.Cmd, bool) {
	if f.rescanBusy {
		return nil, false
	}
	f.rescans++
	return nil, true
}
func (f *fakeScreen) SetDevice(found kbdtui.Found) tea.Cmd {
	f.found = append(f.found, found)
	return nil
}
func (f *fakeScreen) Pending() bool { return f.pending }
func (f *fakeScreen) ArmQuit()      { f.armed = true }

func setup(seen detectMsg) (*model, *fakeScreen, *fakeScreen) {
	mouse, kbd := &fakeScreen{}, &fakeScreen{}
	return newModel(mouse, kbd, seen), mouse, kbd
}

func fkey(n int) tea.KeyMsg {
	if n == 1 {
		return tea.KeyMsg{Type: tea.KeyF1}
	}
	return tea.KeyMsg{Type: tea.KeyF2}
}

var keyboard = kbdtui.Found{Path: "/k", Vendor: 1, Product: 2}

var both = detectMsg{mouseID: "m", mouse: true, kbd: keyboard}

func TestStartScreen(t *testing.T) {
	for _, tt := range []struct {
		seen detectMsg
		want int
	}{
		{both, onMouse},
		{detectMsg{kbd: keyboard}, onKeyboard},
		{detectMsg{}, onMouse},
	} {
		m, _, _ := setup(tt.seen)
		if m.current != tt.want {
			t.Errorf("seen %+v: screen %d", tt.seen, m.current)
		}
	}
}

func TestSwitchAndRouting(t *testing.T) {
	m, mouse, kbd := setup(both)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m.Update(fkey(2))
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if mouse.keys != 1 || kbd.keys != 1 || m.current != onKeyboard {
		t.Fatalf("mouse %d keys, keyboard %d keys, screen %d", mouse.keys, kbd.keys, m.current)
	}
	m.Update(struct{}{})
	if mouse.others != 1 || kbd.others != 1 {
		t.Fatal("a background message did not reach both screens")
	}
}

func TestNoSwitchFromAnOpenEditor(t *testing.T) {
	m, mouse, _ := setup(both)
	mouse.modal = true
	m.Update(fkey(2))
	if m.current != onMouse {
		t.Fatal("switched away from an open editor")
	}
}

func TestDeviceChangesNeverSwitchTheScreen(t *testing.T) {
	m, _, kbd := setup(detectMsg{mouseID: "m", mouse: true})
	m.follow(both)
	if m.current != onMouse || len(kbd.found) != 1 || kbd.found[0] != keyboard {
		t.Fatalf("screen %d, keyboard told %v", m.current, kbd.found)
	}
	m.follow(detectMsg{kbd: keyboard}) // mouse gone
	if m.current != onMouse || len(kbd.found) != 1 {
		t.Fatalf("screen %d, keyboard told %v", m.current, kbd.found)
	}
}

func TestBusyMouseRescanIsRetried(t *testing.T) {
	m, mouse, _ := setup(both)
	mouse.rescanBusy = true
	changed := both
	changed.mouseID = "m2"
	m.follow(changed)
	if mouse.rescans != 0 {
		t.Fatal("rescanned while busy")
	}
	mouse.rescanBusy = false
	m.follow(changed)
	m.follow(changed)
	if mouse.rescans != 1 {
		t.Fatalf("%d rescans, want 1", mouse.rescans)
	}
}

func TestQuitFromMouseStopsAtStagedKeyboardChanges(t *testing.T) {
	m, mouse, kbd := setup(both)
	kbd.pending = true
	m.Update(quitMsg{})
	if m.current != onKeyboard || !kbd.armed || mouse.closed != 0 {
		t.Fatalf("screen %d, armed %v, closed %d", m.current, kbd.armed, mouse.closed)
	}
	if _, cmd := m.Update(quitMsg{}); cmd == nil || mouse.closed != 1 || kbd.closed != 1 {
		t.Fatalf("second quit: closed %d and %d", mouse.closed, kbd.closed)
	}
}

func TestCtrlCClosesBothScreens(t *testing.T) {
	m, mouse, kbd := setup(both)
	mouse.modal = true
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil || mouse.closed != 1 || kbd.closed != 1 {
		t.Fatalf("closed %d and %d", mouse.closed, kbd.closed)
	}
}
