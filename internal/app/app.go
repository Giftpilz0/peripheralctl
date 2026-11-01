// Package app joins the mouse and keyboard screens into one terminal UI.
//
// It detects devices through sysfs only, so watching for them never talks to a
// device. A device appearing never changes the screen.
package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/giftpilz0/peripheralctl/internal/kbdtui"
	"github.com/giftpilz0/peripheralctl/internal/razer"
	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/tui"
)

const detectEvery = 2 * time.Second

const (
	onMouse = iota
	onKeyboard
)

type screen interface {
	tea.Model
	Modal() bool
	Close()
}

type mouseScreen interface {
	screen
	Rescan() (tea.Cmd, bool)
}

type keyboardScreen interface {
	screen
	SetDevice(found kbdtui.Found) tea.Cmd
	Pending() bool
	ArmQuit()
}

type quitMsg struct{}

func requestQuit() tea.Msg { return quitMsg{} }

type detectMsg struct {
	mouseID string
	mouse   bool
	kbd     kbdtui.Found
}

func detect() detectMsg {
	var d detectMsg
	d.mouseID, d.mouse, _ = razer.Present()
	d.kbd, _ = kbdtui.FindKeyboard()
	return d
}

type model struct {
	mouse   mouseScreen
	kbd     keyboardScreen
	current int
	seen    detectMsg
	width   int
}

// newModel opens on the mouse, or the keyboard if only it is connected.
func newModel(mouse mouseScreen, kbd keyboardScreen, seen detectMsg) *model {
	m := &model{mouse: mouse, kbd: kbd, seen: seen}
	if !seen.mouse && seen.kbd.Path != "" {
		m.current = onKeyboard
	}
	return m
}

// Run starts the UI.
func Run() error {
	m := newModel(tui.New(requestQuit), kbdtui.New(requestQuit), detect())
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func tick() tea.Cmd {
	return tea.Tick(detectEvery, func(time.Time) tea.Msg { return detect() })
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.mouse.Init(), m.kbd.Init(), tick()) }

func (m *model) active() screen {
	if m.current == onKeyboard {
		return m.kbd
	}
	return m.mouse
}

// Update routes keys to the visible screen and everything else to both, since
// each screen only reacts to its own messages.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, m.close()
		case "f1", "f2":
			if !m.active().Modal() {
				m.current = onMouse
				if msg.String() == "f2" {
					m.current = onKeyboard
				}
			}
			return m, nil
		}
		_, cmd := m.active().Update(msg)
		return m, cmd
	case quitMsg:
		if m.current == onMouse && m.kbd.Pending() {
			m.current = onKeyboard
			m.kbd.ArmQuit()
			return m, nil
		}
		return m, m.close()
	case detectMsg:
		return m, tea.Batch(m.follow(msg), tick())
	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	_, a := m.mouse.Update(msg)
	_, b := m.kbd.Update(msg)
	return m, tea.Batch(a, b)
}

func (m *model) close() tea.Cmd {
	m.mouse.Close()
	m.kbd.Close()
	return tea.Quit
}

// follow tells a screen its device changed. The mouse screen may be busy; the
// change then stays unseen, so the next detection retries it.
func (m *model) follow(d detectMsg) tea.Cmd {
	var cmds []tea.Cmd
	if d.kbd != m.seen.kbd {
		m.seen.kbd = d.kbd
		cmds = append(cmds, m.kbd.SetDevice(d.kbd))
	}
	m.seen.mouse = d.mouse
	if d.mouseID != m.seen.mouseID {
		if cmd, ok := m.mouse.Rescan(); ok {
			m.seen.mouseID = d.mouseID
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) header() string {
	tab := func(which int, name string, connected bool) string {
		mark := "○"
		if connected {
			mark = "●"
		}
		style := theme.Tab
		if which == m.current {
			style = theme.TabOn
		}
		return style.Render(name + " " + mark)
	}
	return strings.Join([]string{
		theme.Title.Render("peripheralctl"),
		tab(onMouse, "F1 Mouse", m.seen.mouse),
		tab(onKeyboard, "F2 Keyboard", m.seen.kbd.Path != ""),
	}, " ")
}

// View implements tea.Model.
func (m *model) View() string { return m.header() + "\n\n" + m.active().View() }
