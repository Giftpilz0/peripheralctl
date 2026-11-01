// Package kbdtui shows and edits VIA keyboards in a terminal: keymap, backlight
// and saved profiles.
//
// Edits stay in the TUI until the user applies them. Every write is preceded by
// a backup profile and verified by reading the keyboard back. Keyboards off the
// whitelist in package via are shown read-only.
package kbdtui

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/giftpilz0/peripheralctl/internal/colorpick"
	"github.com/giftpilz0/peripheralctl/internal/theme"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// Model is the keyboard screen of the app.
type Model = model

// New creates the screen. quit runs when the user asks to leave.
func New(quit tea.Cmd) *Model {
	m := newModel(nil, &via.Definition{})
	m.quit = quit
	found, err := FindKeyboard()
	if err != nil {
		m.err = err
	}
	m.connect(found)
	return m
}

// connect takes over a keyboard found by FindKeyboard, without reading it.
func (m *model) connect(found Found) {
	m.found = found
	m.dev = nil
	m.def = &via.Definition{}
	if found.Path == "" {
		return
	}
	if def := found.definition(); def != nil {
		m.def = def
		m.dev = m.open(found.Path, def)
	}
	m.row = min(m.row, max(0, len(m.def.Keys)-1))
	if len(m.def.Keys) > 0 {
		m.col = min(m.col, len(m.def.Keys[m.row])-1)
	}
	m.refreshProfiles()
}

// Modal reports an open editor, which must not be left by switching screens.
func (m *model) Modal() bool {
	return m.picker != nil || m.color != nil || m.naming || m.confirm != "" || m.tester != nil
}

// Pending reports staged changes that quitting would drop.
func (m *model) Pending() bool { return m.pending() }

// ArmQuit makes the next quit drop the staged changes, and says so.
func (m *model) ArmQuit() {
	m.quitArmed = true
	m.msg = "unapplied changes: press q again to drop them, or a to apply"
}

// Close takes a colour preview off the LEDs and stops the key tester.
func (m *model) Close() {
	m.stopTester()
	if m.dev == nil || m.state == nil {
		return
	}
	c := m.state.Lighting.Color()
	if m.previewBusy || m.ledShown != c {
		_ = m.dev.Preview(c)
	}
}

// SetDevice follows a keyboard being plugged in or removed; an empty Found means
// it is gone. Edits stay staged for the same keyboard, and a different one drops
// them along with its state.
func (m *model) SetDevice(found Found) tea.Cmd {
	same := m.def != nil && m.def.Matches(found.Vendor, found.Product)
	if found.Path != "" && !same {
		m.edits = map[keyPos]uint16{}
		m.state = nil
		m.layer, m.row, m.col = 0, 0, 0
	}
	if found.Path == "" {
		// Keep the definition, so the last read state stays on screen.
		m.found, m.dev = Found{}, nil
		return nil
	}
	m.err = nil
	m.connect(found)
	if m.dev == nil {
		return nil
	}
	if m.busy != "" {
		m.reload = true
		return nil
	}
	return m.load()
}

type section int

const (
	secKeymap section = iota
	secLighting
	secProfiles
	sectionCount
)

var sectionNames = []string{"Keymap", "Lighting", "Profiles"}

// keyPos addresses one key on one layer.
type keyPos struct{ layer, row, col int }

type stateMsg struct {
	state   *via.State
	changes via.Changes
	applied bool
	done    string
	err     error
}

type model struct {
	quit  tea.Cmd
	open  func(path string, def *via.Definition) backend
	dev   backend
	found Found
	def   *via.Definition

	state  *via.State
	edits  map[keyPos]uint16
	light  via.Lighting // staged backlight; state.Lighting when untouched
	picker *picker
	review []string // what the pending confirmation will write
	reload bool     // the keyboard came back during other work
	tester *tester

	// Colour picker and its preview on the LEDs (colour.go).
	color       *colorpick.Picker
	previewSeq  int
	previewBusy bool
	ledShown    via.Color
	ledWant     via.Color
	afterLED    func() tea.Cmd

	sec           section
	layer         int
	row, col      int
	lightCursor   int
	width, height int

	profiles   []string
	profCursor int
	naming     bool
	nameInput  textinput.Model
	confirm    string // pending y/n question
	onConfirm  func() tea.Cmd

	quitArmed bool
	busy      string
	msg       string
	err       error
	spin      spinner.Model
}

func newModel(dev backend, def *via.Definition) *model {
	m := &model{quit: tea.Quit, open: newDevice, def: def, dev: dev, edits: map[keyPos]uint16{}, spin: spinner.New(), nameInput: textinput.New(), row: 2, col: 1}
	m.spin.Spinner = spinner.MiniDot
	m.nameInput.CharLimit = 64
	m.nameInput.Placeholder = "profile name"
	m.refreshProfiles()
	return m
}

func (m *model) profileDir() string { return profileDir(m.def) }

// readOnly reports a keyboard off the write whitelist.
func (m *model) readOnly() bool { return !m.def.WriteSafe }

func (m *model) refuseWrite() {
	m.msg = fmt.Sprintf("%s is not on the write whitelist, so it is read-only", m.def.Name)
}

func (m *model) refreshProfiles() {
	names, err := listProfiles(m.profileDir())
	if err != nil {
		m.err = err
	}
	m.profiles = names
	m.profCursor = max(0, min(m.profCursor, len(names)-1))
}

func (m *model) Init() tea.Cmd { return m.load() }

func (m *model) start(what string, fn func() stateMsg) tea.Cmd {
	if m.busy != "" || m.dev == nil {
		return nil
	}
	m.busy, m.err, m.msg = what, nil, ""
	return tea.Batch(m.spin.Tick, func() tea.Msg { return fn() })
}

func (m *model) load() tea.Cmd {
	dev := m.dev
	return m.start("reading the keyboard", func() stateMsg {
		s, err := dev.Load()
		return stateMsg{state: s, err: err}
	})
}

// pending reports whether anything is staged.
func (m *model) pending() bool {
	return len(m.edits) > 0 || (m.state != nil && m.light != m.state.Lighting)
}

// staged returns the keyboard state with the staged edits on top.
func (m *model) staged() *via.State {
	want := m.state.Clone()
	want.Lighting = m.light
	for p, code := range m.edits {
		want.Set(p.layer, p.row, p.col, code)
	}
	return want
}

func (m *model) applyStaged() tea.Cmd {
	if m.state == nil || !m.pending() {
		return nil
	}
	if m.readOnly() {
		m.refuseWrite()
		return nil
	}
	if m.dev == nil {
		m.msg = "the keyboard is not connected; the staged changes are kept"
		return nil
	}
	want := m.staged()
	m.review = describe(m.def, m.state, want)
	m.ask(fmt.Sprintf("Write these %d changes to the keyboard? y/n", len(m.review)), func() tea.Cmd {
		// The LEDs first settle on the chosen colour, so nothing lands after the apply.
		apply := func() tea.Cmd { return m.applyState(want, "applied") }
		if !m.previewBusy && m.ledShown == m.ledWant {
			return apply()
		}
		m.afterLED = apply
		return m.syncLED()
	})
	return nil
}

func (m *model) applyState(want *via.State, done string) tea.Cmd {
	dev := m.dev
	saved := m.savedColor()
	return m.start("writing to the keyboard", func() stateMsg {
		s, c, err := dev.Apply(want, saved)
		return stateMsg{state: s, changes: c, applied: true, done: done, err: err}
	})
}

func (m *model) discard() {
	m.edits = map[keyPos]uint16{}
	if m.state != nil {
		m.light = m.state.Lighting
		m.ledWant = m.light.Color()
	}
}

func (m *model) ask(question string, then func() tea.Cmd) {
	m.confirm, m.onConfirm = question, then
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case spinner.TickMsg:
		if m.busy != "" {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	case stateMsg:
		return m, m.handleState(msg)
	case previewTickMsg:
		if int(msg) == m.previewSeq && m.color != nil {
			m.ledWant = pickedColor(m.color)
			return m, m.syncLED()
		}
	case previewDoneMsg:
		return m, m.previewDone(msg)
	case testerMsg:
		return m, m.handleTester(msg)
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleState(msg stateMsg) tea.Cmd {
	m.busy, m.err = "", msg.err
	m.refreshProfiles()
	if msg.state != nil {
		// A reread after a reconnect keeps what is staged.
		keep := !msg.applied && m.pending()
		m.state = msg.state
		m.layer = min(m.layer, m.state.Layers-1)
		// A failed write keeps the edits, so it can be retried.
		switch {
		case msg.err != nil || keep:
			m.dropAppliedEdits()
		default:
			m.discard()
		}
		c := m.state.Lighting.Color()
		m.ledShown, m.ledWant = c, c
	}
	if msg.err == nil && msg.applied {
		m.msg = fmt.Sprintf("%s and verified: %s", msg.done, msg.changes)
	} else if msg.err == nil && msg.done != "" {
		m.msg = msg.done
	}
	if msg.err != nil && msg.applied {
		m.err = fmt.Errorf("%w; the previous state is in profile %s", msg.err, autoProfile)
	}
	if m.reload {
		m.reload = false
		return m.load()
	}
	return nil
}

func (m *model) dropAppliedEdits() {
	for p, code := range m.edits {
		if m.state.At(p.layer, p.row, p.col) == code {
			delete(m.edits, p)
		}
	}
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case m.busy != "":
		return nil
	case m.confirm != "":
		then := m.onConfirm
		m.confirm, m.onConfirm, m.review = "", nil, nil
		if msg.String() == "y" {
			return then()
		}
		return nil
	case m.tester != nil:
		if msg.String() == "esc" {
			m.stopTester()
		}
		return nil
	case m.color != nil:
		return m.updateColor(msg)
	case m.picker != nil:
		m.updatePicker(msg)
		return nil
	case m.naming:
		return m.updateNaming(msg)
	}

	key := msg.String()
	if key == "q" {
		if m.pending() && !m.quitArmed {
			m.ArmQuit()
			return nil
		}
		return m.quit
	}
	m.quitArmed = false
	m.msg = ""

	switch key {
	case "tab":
		m.sec = (m.sec + 1) % sectionCount
		return nil
	case "shift+tab":
		m.sec = (m.sec + sectionCount - 1) % sectionCount
		return nil
	case "1", "2", "3":
		m.sec = section(key[0] - '1')
		return nil
	case "r":
		if m.pending() {
			m.msg = "apply or discard the staged changes first"
			return nil
		}
		return m.load()
	case "a":
		return m.applyStaged()
	case "u":
		m.discard()
		return m.syncLED()
	}

	switch m.sec {
	case secKeymap:
		return m.updateKeymap(key)
	case secLighting:
		m.updateLighting(key)
	case secProfiles:
		return m.updateProfiles(key)
	}
	return nil
}

func (m *model) updateKeymap(key string) tea.Cmd {
	if len(m.def.Keys) == 0 {
		return nil
	}
	switch key {
	case "t":
		return m.startTester()
	case "left", "h":
		m.col = max(0, m.col-1)
	case "right", "l":
		m.col = min(len(m.def.Keys[m.row])-1, m.col+1)
	case "up", "k":
		m.moveRow(-1)
	case "down", "j":
		m.moveRow(1)
	case "]", "n":
		m.shiftLayer(1)
	case "[", "p":
		m.shiftLayer(-1)
	case "enter":
		if m.readOnly() {
			m.refuseWrite()
		} else if m.state != nil {
			m.picker = newPicker(m.state, m.def, m.def.Keys[m.row][m.col])
		}
	case "backspace", "delete":
		k := m.def.Keys[m.row][m.col]
		delete(m.edits, keyPos{m.layer, k.Row, k.Col})
	}
	return nil
}

// moveRow keeps the cursor at the same horizontal position on the board.
func (m *model) moveRow(dir int) {
	to := m.row + dir
	if to < 0 || to >= len(m.def.Keys) {
		return
	}
	cur := m.def.Keys[m.row][m.col]
	m.col = nearest(m.def.Keys[to], cur.X+cur.W/2)
	m.row = to
}

func nearest(row []via.Key, x float64) int {
	best, dist := 0, math.Inf(1)
	for i, k := range row {
		if d := math.Abs(k.X + k.W/2 - x); d < dist {
			best, dist = i, d
		}
	}
	return best
}

func (m *model) shiftLayer(dir int) {
	if m.state == nil {
		return
	}
	m.layer = (m.layer + dir + m.state.Layers) % m.state.Layers
}

func (m *model) updateNaming(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.naming = false
		return nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if err := checkName(name); err != nil {
			m.err = err
			return nil
		}
		m.naming = false
		save := func() tea.Cmd {
			dev, dir := m.dev, m.profileDir()
			return m.start("saving the profile", func() stateMsg {
				s, err := dev.Load()
				if err == nil {
					err = saveProfile(dir, name, s)
				}
				return stateMsg{state: s, done: "saved profile " + name, err: err}
			})
		}
		if slices.Contains(m.profiles, name) {
			m.ask(fmt.Sprintf("Overwrite profile %s? y/n", name), save)
			return nil
		}
		return save()
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return cmd
}

func (m *model) updateProfiles(key string) tea.Cmd {
	switch key {
	case "up", "k":
		m.profCursor = max(0, m.profCursor-1)
	case "down", "j":
		m.profCursor = min(max(0, len(m.profiles)-1), m.profCursor+1)
	case "s":
		switch {
		case m.state == nil:
			m.err = errors.New("the keyboard has not been read")
		case m.pending():
			m.msg = "apply or discard the staged changes first; a profile stores what is on the keyboard"
		default:
			m.err = nil
			m.naming = true
			m.nameInput.SetValue("")
			return m.nameInput.Focus()
		}
	case "enter":
		return m.askRestore()
	case "d":
		if len(m.profiles) == 0 {
			return nil
		}
		name := m.profiles[m.profCursor]
		m.ask(fmt.Sprintf("Delete profile %s? y/n", name), func() tea.Cmd {
			m.err = deleteProfile(m.profileDir(), name)
			m.refreshProfiles()
			return nil
		})
	}
	return nil
}

func (m *model) askRestore() tea.Cmd {
	if len(m.profiles) == 0 || m.state == nil {
		return nil
	}
	if m.readOnly() {
		m.refuseWrite()
		return nil
	}
	if m.pending() {
		m.msg = "apply or discard the staged changes first"
		return nil
	}
	name := m.profiles[m.profCursor]
	want, err := loadProfile(m.profileDir(), name)
	if err != nil {
		m.err = err
		return nil
	}
	if err := m.state.Compatible(want); err != nil {
		m.err = fmt.Errorf("profile %s does not fit: %w", name, err)
		return nil
	}
	changes := m.state.Diff(want)
	if changes.None() {
		m.msg = "the keyboard already matches " + name
		return nil
	}
	m.review = describe(m.def, m.state, want)
	m.ask(fmt.Sprintf("Restore %s? Changes %s. y/n", name, changes), func() tea.Cmd {
		return m.applyState(want, "restored "+name)
	})
	return nil
}

// ---- view ------------------------------------------------------------------

var (
	sKey    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.DimColor).Align(lipgloss.Center)
	sKeyDim = sKey.Foreground(theme.DimColor)
	sKeyEd  = sKey.BorderForeground(theme.WarnColor).Foreground(theme.WarnColor).Bold(true)
	sKeyOn  = sKey.BorderForeground(theme.AccentColor).Foreground(theme.AccentColor).Bold(true)
	sKeyHit = sKey.BorderForeground(theme.AccentColor).Background(theme.AccentColor).Foreground(lipgloss.Color("#000000")).Bold(true)
)

// scale picks the widest key size that fits; a key needs 4 columns to show a
// short label inside its border.
func (m *model) scale() int {
	for _, s := range []int{7, 6, 5} {
		if float64(m.width) >= m.def.Width()*float64(s) {
			return s
		}
	}
	return 4
}

func truncate(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return string([]rune(s)[:max(0, n)])
}

// code returns a key's keycode with staged edits on top.
func (m *model) code(k via.Key) uint16 {
	if c, ok := m.edits[keyPos{m.layer, k.Row, k.Col}]; ok {
		return c
	}
	return m.state.At(m.layer, k.Row, k.Col)
}

func (m *model) label(k via.Key) string {
	if m.state == nil {
		return k.Stock
	}
	return via.Label(m.state.Protocol, m.code(k), m.def.Custom)
}

func (m *model) renderKey(k via.Key, on bool, width int) string {
	label := m.label(k)
	inner := width - 2
	style := sKey
	_, edited := m.edits[keyPos{m.layer, k.Row, k.Col}]
	switch {
	case m.tester != nil && m.state != nil && m.tester.pressed[m.code(k)]:
		style = sKeyHit
	case on && m.tester == nil:
		style = sKeyOn
	case edited:
		style = sKeyEd
	case m.state != nil && (label == "" || label == "▽" || label == "fw"):
		style = sKeyDim
	}
	return style.Width(inner).Render(truncate(label, inner))
}

func (m *model) board(s int) string {
	cols := func(u float64) int { return int(math.Round(u * float64(s))) }
	var lines []string
	for r, row := range m.def.Keys {
		parts := []string{}
		end := 0
		for c, k := range row {
			left, right := cols(k.X), cols(k.X+k.W)
			if left > end {
				parts = append(parts, strings.Repeat(" ", left-end))
			}
			parts = append(parts, m.renderKey(k, r == m.row && c == m.col, right-left))
			end = right
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
		if r == 0 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func tabs(names []string, cur int) string {
	out := make([]string, len(names))
	for i, name := range names {
		style := theme.Tab
		if i == cur {
			style = theme.TabOn
		}
		out[i] = style.Render(name)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, out...)
}

func (m *model) layerTabs() string {
	if m.state == nil {
		return theme.TabOn.Render("Stock layout") + theme.Dim.Render(" not read from the keyboard")
	}
	names := make([]string, m.state.Layers)
	for i := range names {
		names[i] = fmt.Sprintf("Layer %d", i)
	}
	return tabs(names, m.layer)
}

func (m *model) keyDetail() string {
	k := m.def.Keys[m.row][m.col]
	if m.state == nil {
		return fmt.Sprintf("%s  %s", theme.Title.Render(k.Stock), theme.Dim.Render(fmt.Sprintf("matrix %d,%d", k.Row, k.Col)))
	}
	code := m.code(k)
	label := m.label(k)
	switch label {
	case "":
		label = "no action"
	case "▽":
		label = "▽ falls through to the layer below"
	case "fw":
		label = "no VIA keycode, probably handled by the firmware"
	}
	line := fmt.Sprintf("%s  %s", theme.Title.Render(label),
		theme.Dim.Render(fmt.Sprintf("keycode %#04x · matrix %d,%d · position %s", code, k.Row, k.Col, k.Stock)))
	if _, edited := m.edits[keyPos{m.layer, k.Row, k.Col}]; edited {
		line += theme.Warn.Render(fmt.Sprintf(" · staged, on the keyboard: %s",
			via.Label(m.state.Protocol, m.state.At(m.layer, k.Row, k.Col), m.def.Custom)))
	}
	return line
}

func (m *model) keymapView() string {
	if len(m.def.Keys) == 0 {
		return theme.Dim.Render("no keyboard to show")
	}
	detail := m.keyDetail()
	if m.tester != nil {
		detail = m.tester.view()
	}
	parts := []string{m.layerTabs(), "", m.board(m.scale()), "", detail}
	if m.picker != nil {
		parts = append(parts, "", m.picker.view())
	}
	return strings.Join(parts, "\n")
}

func (m *model) profilesView() string {
	var b strings.Builder
	b.WriteString(theme.Dim.Render("A profile holds the keymap and backlight, the two things VIA can read back.") + "\n\n")
	if len(m.profiles) == 0 {
		b.WriteString(theme.Dim.Render("no saved profiles") + "\n")
	}
	for i, name := range m.profiles {
		switch {
		case i == m.profCursor:
			b.WriteString(theme.Title.Render("› "+name) + "\n")
		case name == autoProfile:
			b.WriteString("  " + theme.Dim.Render(name+" (automatic)") + "\n")
		default:
			b.WriteString("  " + name + "\n")
		}
	}
	if m.naming {
		b.WriteString("\nSave the keyboard's current state as: " + m.nameInput.View() + "\n")
	}
	return b.String()
}

func (m *model) status() string {
	switch {
	case m.busy != "":
		return m.spin.View() + " " + m.busy + "…"
	case m.confirm != "":
		return theme.Warn.Render(m.confirm)
	case m.found.Path != "" && m.dev == nil:
		return theme.Warn.Render(fmt.Sprintf("VIA keyboard %04x:%04x has no definition. Get its VIA JSON from the maker, then run: peripheralctl import FILE", m.found.Vendor, m.found.Product))
	case m.dev == nil && m.state != nil:
		return theme.Warn.Render("keyboard not connected: showing the last read state, staged changes are kept")
	case m.dev == nil && m.err == nil:
		return theme.Warn.Render("no VIA keyboard connected")
	case errors.Is(m.err, via.ErrNoAccess):
		return theme.Warn.Render(m.err.Error() + ": run 'peripheralctl install' to add the udev rule")
	case m.err != nil:
		return theme.Bad.Render(m.err.Error())
	case m.msg != "":
		return theme.Warn.Render(m.msg)
	case m.state == nil:
		return theme.Dim.Render("not read yet")
	}
	if m.pending() {
		s := m.state.Diff(m.staged())
		return theme.Warn.Render("staged, not on the keyboard yet: " + s.String() + " · a apply · u discard")
	}
	return theme.Dim.Render(fmt.Sprintf("%s · VIA protocol %d · %d layers · in sync", m.found.Path, m.state.Protocol, m.state.Layers))
}

func (m *model) help() string {
	switch {
	case m.tester != nil:
		return "press keys to see what the computer receives · esc stop · ctrl+c quit"
	case m.color != nil:
		return m.color.Help() + " · shown on the LEDs after a short pause"
	case m.picker != nil:
		return "type to filter or enter 0x… · ↑↓ choose · enter stage · esc cancel"
	case m.naming:
		return "enter save · esc cancel"
	}
	common := "tab section · a apply · u discard · r reread · q quit"
	switch m.sec {
	case secLighting:
		return "↑↓ field · ←→ change · enter pick colour · " + common
	case secProfiles:
		return "↑↓ choose · s save current · enter restore · d delete · " + common
	}
	return "←↑↓→ move · [ ] layer · enter remap · backspace undo key · t key test · " + common
}

// View implements tea.Model.
func (m *model) View() string {
	if m.width > 0 && float64(m.width) < m.def.Width()*4 {
		return theme.Warn.Render(fmt.Sprintf("terminal too narrow: need %d columns", int(math.Ceil(m.def.Width()*4))))
	}
	var body string
	switch {
	case m.review != nil:
		body = m.reviewView()
	case m.sec == secLighting:
		body = m.lightingView()
	case m.sec == secProfiles:
		body = m.profilesView()
	default:
		body = m.keymapView()
	}
	name := m.def.Name
	if name == "" {
		name = "Keyboard"
	}
	warning := ""
	if m.dev != nil && m.readOnly() {
		warning = theme.Bad.Render("⚠ Not on the write whitelist: shown read-only, nothing is written to it.") + "\n"
	}
	return strings.Join([]string{
		theme.Title.Render(name) + " " + tabs(sectionNames, int(m.sec)),
		warning,
		body,
		"",
		m.status(),
		theme.Dim.Render(m.help()),
	}, "\n")
}
