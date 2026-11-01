package kbdtui

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/giftpilz0/peripheralctl/internal/config"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// fakeBackend is an in-memory keyboard that records every Apply.
type fakeBackend struct {
	state      *via.State
	applied    []*via.State
	previews   []via.Color
	savedColor *via.Color
}

func crush() *via.Definition { return via.Lookup(via.Crush80.Vendor, via.Crush80.Products[0], nil) }

func testState() *via.State {
	d := crush()
	s := &via.State{
		Protocol: 11, Layers: 4, Rows: d.Rows, Cols: d.Cols,
		Codes:    make([]uint16, 4*d.Rows*d.Cols),
		Lighting: via.Lighting{Brightness: 4, Effect: 6, Speed: 2, Hue: 20, Saturation: 255},
	}
	s.Set(0, 2, 1, 0x0014) // Q
	return s
}

func (f *fakeBackend) Load() (*via.State, error) { return f.state.Clone(), nil }

func (f *fakeBackend) Preview(c via.Color) error {
	f.previews = append(f.previews, c)
	f.state.Lighting.Hue, f.state.Lighting.Saturation = c.Hue, c.Saturation
	return nil
}

func (f *fakeBackend) Apply(want *via.State, saved *via.Color) (*via.State, via.Changes, error) {
	f.applied = append(f.applied, want.Clone())
	f.savedColor = saved
	c := f.state.Diff(want)
	if saved != nil {
		cur := f.state.Clone()
		cur.Lighting.Hue, cur.Lighting.Saturation = saved.Hue, saved.Saturation
		c = cur.Diff(want)
	}
	f.state = want.Clone()
	return f.state.Clone(), c, nil
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press sends keys and runs what they start, as the Bubble Tea loop would.
func press(m *model, keys ...string) {
	for _, s := range keys {
		_, cmd := m.Update(keyMsg(s))
		run(m, cmd)
	}
}

func run(m *model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
	case stateMsg, previewDoneMsg:
		_, next := m.Update(msg)
		run(m, next)
	}
}

func loaded(t *testing.T) (*model, *fakeBackend) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f := &fakeBackend{state: testState()}
	m := newModel(f, crush())
	run(m, m.Init())
	if m.state == nil {
		t.Fatalf("not loaded: %v", m.err)
	}
	return m, f
}

func TestBoardFitsWidth(t *testing.T) {
	for _, width := range []int{84, 104, 125, 146, 200} {
		m := newModel(nil, crush())
		m.width = width
		for line := range strings.SplitSeq(m.board(m.scale()), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: line is %d wide", width, got)
			}
		}
	}
}

func TestMoveRowKeepsPosition(t *testing.T) {
	m := newModel(nil, crush())
	m.row, m.col = 1, 14 // Ins
	press(m, "down")
	if got := m.def.Keys[m.row][m.col].Stock; got != "Del" {
		t.Fatalf("down from Ins = %s", got)
	}
	m.row, m.col = 5, 3 // Space
	press(m, "up")
	if got := m.def.Keys[m.row][m.col].Stock; got != "B" {
		t.Fatalf("up from Space = %s", got)
	}
}

func TestRemapStaysStagedUntilApplied(t *testing.T) {
	m, f := loaded(t)
	if got := m.def.Keys[m.row][m.col].Stock; got != "Q" {
		t.Fatalf("cursor on %s", got)
	}
	press(m, "enter", "F", "1", "3", "enter")
	if m.picker != nil {
		t.Fatal("picker still open")
	}
	if len(f.applied) != 0 {
		t.Fatal("remapping wrote to the keyboard")
	}
	if got := m.label(m.def.Keys[m.row][m.col]); got != "F13" {
		t.Fatalf("label %q", got)
	}
	if !m.pending() {
		t.Fatal("nothing staged")
	}
	press(m, "a")
	if len(f.applied) != 0 || len(m.review) != 1 {
		t.Fatalf("a wrote without a review: %d applies, review %v", len(f.applied), m.review)
	}
	if want := "Layer 0 · Q: Q → F13"; m.review[0] != want {
		t.Fatalf("review %q, want %q", m.review[0], want)
	}
	press(m, "y")
	if len(f.applied) != 1 {
		t.Fatalf("%d applies", len(f.applied))
	}
	if got := f.state.At(0, 2, 1); got != 0x0068 {
		t.Fatalf("keycode %#x", got)
	}
	if m.pending() {
		t.Fatal("still staged after apply")
	}
}

func TestDiscardDropsEdits(t *testing.T) {
	m, f := loaded(t)
	press(m, "enter", "0", "x", "0", "0", "6", "8", "enter", "u", "a")
	if m.pending() || len(f.applied) != 0 {
		t.Fatalf("pending %v, %d applies", m.pending(), len(f.applied))
	}
}

func TestPickerRefusesDangerousKeycodes(t *testing.T) {
	m, f := loaded(t)
	press(m, "enter", "0", "x", "7", "c", "0", "3", "enter")
	if m.picker == nil || m.picker.problem == "" {
		t.Fatal("EEPROM keycode was accepted")
	}
	press(m, "esc", "a")
	if m.pending() || len(f.applied) != 0 {
		t.Fatal("dangerous keycode reached the keyboard")
	}
}

func TestLightingStaysStagedUntilApplied(t *testing.T) {
	m, f := loaded(t)
	press(m, "2", "right")
	if m.light.Brightness != 5 || f.state.Lighting.Brightness != 4 {
		t.Fatalf("staged %d, keyboard %d", m.light.Brightness, f.state.Lighting.Brightness)
	}
	press(m, "a", "y")
	if f.state.Lighting.Brightness != 5 || m.pending() {
		t.Fatal("not applied")
	}
}

func TestLightingLimits(t *testing.T) {
	m, _ := loaded(t)
	press(m, "2")
	for range 20 {
		press(m, "right")
	}
	if int(m.light.Brightness) != m.def.MaxBrightness {
		t.Fatalf("brightness %d", m.light.Brightness)
	}
	press(m, "down")
	for range 40 {
		press(m, "right")
	}
	if int(m.light.Effect) >= len(m.def.Effects) {
		t.Fatalf("effect %d", m.light.Effect)
	}
}

func TestLEDShortcutsFollowTheKeymap(t *testing.T) {
	m, _ := loaded(t)
	m.state.Set(1, 2, 11, 0x7f07) // logo effect on Fn+[
	shortcuts := func() map[string]string {
		got := map[string]string{}
		for _, s := range m.ledShortcuts() {
			got[s.label] = s.keys
		}
		return got
	}
	got := shortcuts()
	if got["Logo effect"] != "Fn+[" || got["Side on/off"] != "no key assigned" {
		t.Fatalf("shortcuts %v", got)
	}
	m.edits[keyPos{0, 2, 1}] = 0x7f0c // staged: side on/off on Q
	if got := shortcuts()["Side on/off"]; got != "Q" {
		t.Fatalf("staged shortcut %q", got)
	}
}

func TestQuitAsksWhenStaged(t *testing.T) {
	m, _ := loaded(t)
	press(m, "2", "right")
	if _, cmd := m.Update(keyMsg("q")); cmd != nil {
		t.Fatal("first q quit with staged changes")
	}
	if _, cmd := m.Update(keyMsg("q")); cmd == nil {
		t.Fatal("second q did not quit")
	}
}

func TestProfileSaveAndRestore(t *testing.T) {
	m, f := loaded(t)
	press(m, "3", "s", "m", "i", "n", "e", "enter")
	if len(m.profiles) != 1 || m.profiles[0] != "mine" {
		t.Fatalf("profiles %v, err %v", m.profiles, m.err)
	}
	if len(f.applied) != 0 {
		t.Fatal("saving wrote to the keyboard")
	}

	original := f.state.Clone()
	f.state.Lighting.Brightness = 9
	f.state.Set(1, 0, 0, 0x0004)
	press(m, "r", "enter")
	if m.confirm == "" || len(f.applied) != 0 {
		t.Fatalf("restore did not ask first: %q", m.confirm)
	}
	press(m, "n")
	if len(f.applied) != 0 {
		t.Fatal("declined restore still wrote")
	}
	press(m, "enter")
	if len(m.review) != 2 {
		t.Fatalf("review %v", m.review)
	}
	press(m, "y")
	if len(f.applied) != 1 || !f.state.Equal(original) {
		t.Fatalf("%d applies, state restored %v, err %v", len(f.applied), f.state.Equal(original), m.err)
	}
	press(m, "enter")
	if len(f.applied) != 1 {
		t.Fatal("restoring an identical profile wrote again")
	}
}

func TestProfileNames(t *testing.T) {
	for name, ok := range map[string]bool{
		"mine": true, "a.b_c-1": true,
		"": false, "../evil": false, "a/b": false, ".hidden": false, strings.Repeat("x", 65): false,
	} {
		if err := checkName(name); (err == nil) != ok {
			t.Errorf("checkName(%q) = %v", name, err)
		}
	}
}

func TestProfileFileRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := testState()
	s.Set(1, 2, 3, 0x0041)
	dir := t.TempDir()
	if err := saveProfile(dir, "x", s); err != nil {
		t.Fatal(err)
	}
	got, err := loadProfile(dir, "x")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(s) {
		t.Fatal("profile differs after a round trip")
	}
}

func TestReviewDeclineWritesNothing(t *testing.T) {
	m, f := loaded(t)
	press(m, "2", "right", "a")
	if !strings.Contains(strings.Join(m.review, "\n"), "Brightness: 4 → 5") {
		t.Fatalf("review %v", m.review)
	}
	press(m, "n")
	if len(f.applied) != 0 || m.review != nil || !m.pending() {
		t.Fatalf("%d applies, review %v, pending %v", len(f.applied), m.review, m.pending())
	}
}

func TestHueBytesSurviveThePicker(t *testing.T) {
	for hue := range 256 {
		c := via.Color{Hue: byte(hue), Saturation: 255}
		if got := pickedColor(colorPickerFor(c)); got != c {
			t.Fatalf("%v came back as %v", c, got)
		}
	}
	for sat := range 256 {
		c := via.Color{Hue: 170, Saturation: byte(sat)}
		if got := pickedColor(colorPickerFor(c)); got != c {
			t.Fatalf("%v came back as %v", c, got)
		}
	}
}

// pickColor opens the picker, moves the hue and lets the pause pass.
func pickColor(m *model, keys ...string) {
	press(m, "2", "down", "down", "down", "enter")
	press(m, keys...)
	_, cmd := m.Update(previewTickMsg(m.previewSeq))
	run(m, cmd)
}

func TestColorPreviewOnlyAfterAPause(t *testing.T) {
	m, f := loaded(t)
	press(m, "2", "down", "down", "down", "enter")
	first := m.previewSeq
	press(m, "right", "right", "right")
	if len(f.previews) != 0 {
		t.Fatalf("%d previews during the drag", len(f.previews))
	}
	m.Update(previewTickMsg(first)) // the tick of an older step
	if len(f.previews) != 0 || m.previewBusy {
		t.Fatal("an outdated tick sent a preview")
	}
	_, cmd := m.Update(previewTickMsg(m.previewSeq))
	run(m, cmd)
	if len(f.previews) != 1 {
		t.Fatalf("%d previews, want 1", len(f.previews))
	}
}

func TestColorCancelRestoresTheLEDs(t *testing.T) {
	m, f := loaded(t)
	before := f.state.Lighting.Color()
	pickColor(m, "right")
	press(m, "esc")
	if got := f.previews[len(f.previews)-1]; got != before || len(f.previews) != 2 {
		t.Fatalf("previews %v, want the last to be %v", f.previews, before)
	}
	if m.pending() {
		t.Fatal("cancel staged a colour")
	}
}

func TestColorCancelWithoutPreviewWritesNothing(t *testing.T) {
	m, f := loaded(t)
	press(m, "2", "down", "down", "down", "enter", "right", "esc")
	if len(f.previews) != 0 {
		t.Fatalf("previews %v", f.previews)
	}
}

func TestPreviewedColorIsSavedOnApply(t *testing.T) {
	m, f := loaded(t)
	before := f.state.Lighting.Color()
	pickColor(m, "right")
	press(m, "enter")
	picked := m.light.Color()
	if picked == before || !m.pending() {
		t.Fatalf("not staged: %v", picked)
	}
	press(m, "a", "y")
	if len(f.applied) != 1 {
		t.Fatalf("%d applies", len(f.applied))
	}
	if f.savedColor == nil || *f.savedColor != before {
		t.Fatalf("saved colour %v, want %v, or the keyboard would skip saving", f.savedColor, before)
	}
	if f.state.Lighting.Color() != picked || m.pending() {
		t.Fatal("colour not applied")
	}
}

func TestDiscardPutsTheLEDsBack(t *testing.T) {
	m, f := loaded(t)
	before := f.state.Lighting.Color()
	pickColor(m, "right")
	press(m, "enter", "u")
	if got := f.previews[len(f.previews)-1]; got != before {
		t.Fatalf("LEDs left on %v", got)
	}
}

func TestCloseTakesPreviewOffTheLEDs(t *testing.T) {
	m, f := loaded(t)
	before := f.state.Lighting.Color()
	pickColor(m, "right")
	m.Close()
	if got := f.previews[len(f.previews)-1]; got != before {
		t.Fatalf("LEDs left on %v", got)
	}
	n := len(f.previews)
	m2, f2 := loaded(t)
	m2.Close()
	if len(f2.previews) != 0 || n == 0 {
		t.Fatal("close without a preview wrote")
	}
}

func crushAt(path string) Found { return Found{path, via.Crush80.Vendor, via.Crush80.Products[0]} }

func TestDisconnectKeepsEditsAndReconnectRereads(t *testing.T) {
	m, f := loaded(t)
	press(m, "2", "right")
	m.SetDevice(Found{})
	if !m.pending() {
		t.Fatal("disconnect dropped the staged change")
	}
	press(m, "a")
	if m.confirm != "" || len(f.applied) != 0 {
		t.Fatal("apply went ahead without a keyboard")
	}
	fresh := &fakeBackend{state: testState()}
	m.open = func(string, *via.Definition) backend { return fresh }
	run(m, m.SetDevice(crushAt("/dev/hidraw9")))
	if m.dev != backend(fresh) || !m.pending() || m.light.Brightness != 5 {
		t.Fatalf("after reconnect: pending %v, brightness %d", m.pending(), m.light.Brightness)
	}
	press(m, "a", "y")
	if len(fresh.applied) != 1 {
		t.Fatalf("%d applies on the reconnected keyboard", len(fresh.applied))
	}
}

func TestReconnectDuringWorkRereadsAfterIt(t *testing.T) {
	m, _ := loaded(t)
	fresh := &fakeBackend{state: testState()}
	fresh.state.Set(0, 2, 1, 0x0004)
	m.open = func(string, *via.Definition) backend { return fresh }
	m.busy = "reading the keyboard"
	if cmd := m.SetDevice(crushAt("/dev/hidraw9")); cmd != nil {
		t.Fatal("started a second read during work")
	}
	_, cmd := m.Update(stateMsg{state: testState()})
	run(m, cmd)
	if m.state.At(0, 2, 1) != 0x0004 {
		t.Fatal("did not reread after the work ended")
	}
}

func TestTesterSwallowsKeysUntilEsc(t *testing.T) {
	m, f := loaded(t)
	m.tester = &tester{pressed: map[uint16]bool{}, stop: make(chan struct{})}
	press(m, "2", "a", "q", "enter")
	if m.sec != secKeymap || len(f.applied) != 0 {
		t.Fatal("a key reached the UI during the test")
	}
	press(m, "esc")
	if m.tester != nil {
		t.Fatal("esc did not stop the test")
	}
}

func TestTesterHighlightsMatchingKey(t *testing.T) {
	m, _ := loaded(t)
	tr := &tester{pressed: map[uint16]bool{}, stop: make(chan struct{})}
	m.tester = tr
	m.handleTester(testerMsg{t: tr, ev: inputEvent{scan: 0x70014, down: true}, ok: true})
	if !tr.pressed[0x14] || !strings.Contains(tr.last, "Q") {
		t.Fatalf("pressed %v, last %q", tr.pressed, tr.last)
	}
	m.handleTester(testerMsg{t: tr, ev: inputEvent{scan: 0x70014}, ok: true})
	if tr.pressed[0x14] {
		t.Fatal("still pressed after release")
	}
}

func TestParseEvent(t *testing.T) {
	event := func(typ, code uint16, value int32) []byte {
		b := make([]byte, inputEventSize)
		off := inputEventSize - 8
		binary.NativeEndian.PutUint16(b[off:], typ)
		binary.NativeEndian.PutUint16(b[off+2:], code)
		binary.NativeEndian.PutUint32(b[off+4:], uint32(value))
		return b
	}
	var scan uint32
	if _, ok := parseEvent(event(evMsc, mscScan, 0x70014), &scan); ok {
		t.Fatal("a scan event counted as a key")
	}
	ev, ok := parseEvent(event(evKey, 16, 1), &scan)
	if !ok || !ev.down || ev.code != 16 || ev.scan != 0x70014 {
		t.Fatalf("press %+v, %v", ev, ok)
	}
	if _, ok := parseEvent(event(evKey, 16, 2), &scan); ok {
		t.Fatal("auto-repeat counted as a press")
	}
	if ev, ok := parseEvent(event(evKey, 16, 0), &scan); !ok || ev.down {
		t.Fatalf("release %+v, %v", ev, ok)
	}
}

func TestInputNodesFindsOnlyTheKeyboard(t *testing.T) {
	root := t.TempDir()
	add := func(name, vendor, product string) {
		dir := filepath.Join(root, name, "device", "id")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for file, v := range map[string]string{"vendor": vendor, "product": product} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("event1", "320f", "5055")
	add("event2", "1532", "007d")
	add("event3", "320f", "5088")
	add("event4", "320f", "9999")
	got := inputNodes(root, crush())
	if len(got) != 2 || got[0] != "/dev/input/event1" || got[1] != "/dev/input/event3" {
		t.Fatalf("nodes %v", got)
	}
}

func TestUsageCode(t *testing.T) {
	for scan, want := range map[uint32]uint16{0x70004: 0x04, 0x700e0: 0xe0, 0xc00e9: 0xa9, 0xc00e2: 0xa8} {
		if got, ok := usageCode(scan); !ok || got != want {
			t.Errorf("usage %#x = %#x, %v", scan, got, ok)
		}
	}
	if _, ok := usageCode(0x90001); ok {
		t.Error("a mouse button matched a keycode")
	}
}

func readOnlyModel(t *testing.T) (*model, *fakeBackend) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	def := crush()
	def.WriteSafe = false
	f := &fakeBackend{state: testState()}
	m := newModel(f, def)
	run(m, m.Init())
	return m, f
}

func TestReadOnlyKeyboardCannotBeEdited(t *testing.T) {
	m, f := readOnlyModel(t)
	if !strings.Contains(m.View(), "write whitelist") {
		t.Fatal("no warning on screen")
	}
	press(m, "enter")
	if m.picker != nil {
		t.Fatal("the remap picker opened")
	}
	press(m, "2", "right", "down", "down", "down", "enter")
	if m.color != nil || m.pending() {
		t.Fatalf("lighting editable: picker %v, pending %v", m.color != nil, m.pending())
	}
	press(m, "a", "y")
	if len(f.applied) != 0 || len(f.previews) != 0 {
		t.Fatalf("%d applies, %d previews", len(f.applied), len(f.previews))
	}
}

func TestReadOnlyKeyboardCanBeSavedNotRestored(t *testing.T) {
	m, f := readOnlyModel(t)
	press(m, "3", "s", "m", "i", "n", "e", "enter")
	if len(m.profiles) != 1 {
		t.Fatalf("profiles %v, err %v", m.profiles, m.err)
	}
	f.state.Set(0, 0, 0, 0x0004)
	press(m, "r", "enter", "y")
	if len(f.applied) != 0 || m.review != nil {
		t.Fatalf("%d applies, review %v", len(f.applied), m.review)
	}
}

func TestKeyboardWithoutDefinitionIsExplained(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel(nil, &via.Definition{})
	m.connect(Found{"/dev/hidraw9", 0x1234, 0x0001})
	if m.dev != nil {
		t.Fatal("opened a keyboard without a definition")
	}
	if !strings.Contains(m.status(), "1234:0001") || !strings.Contains(m.status(), "import") {
		t.Fatalf("status %q", m.status())
	}
	m.View()
	press(m, "enter", "t", "2", "right")
}

func TestImportedDefinitionsStayReadOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "pad.json")
	text := `{"name":"My Pad","vendorId":"0xFEED","productId":"0x0001","matrix":{"rows":1,"cols":2},"layouts":{"keymap":[["0,0","0,1"]]}}`
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := via.ImportDefinition(file, config.KeyboardsDir()); err != nil {
		t.Fatal(err)
	}
	def := (Found{"/dev/x", 0xfeed, 0x0001}).definition()
	if def == nil || def.Name != "My Pad" || def.WriteSafe {
		t.Fatalf("%+v", def)
	}
}

func TestImportRefusesBuiltin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "c.json")
	text := `{"name":"Fake","vendorId":"0x320F","productId":"0x5055","matrix":{"rows":1,"cols":2},"layouts":{"keymap":[["0,0"]]}}`
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := via.ImportDefinition(file, config.KeyboardsDir()); err == nil {
		t.Fatal("a file replaced a built-in definition")
	}
}

func TestProfilesAreKeptPerKeyboard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if profileDir(crush()) == profileDir(&via.Definition{Name: "Other"}) {
		t.Fatal("two keyboards share a profile folder")
	}
}
