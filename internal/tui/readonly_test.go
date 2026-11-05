package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/giftpilz0/peripheralctl/internal/razer"
)

// A zero Conn has no device, so reaching it would panic.
func withMouse(t *testing.T, model razer.Model) *model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.found.Mouse = &razer.Mouse{Conn: &razer.Conn{}, Model: model}
	return m
}

func modelNamed(t *testing.T, name string) razer.Model {
	t.Helper()
	for _, model := range razer.Models {
		if model.Name == name {
			return model
		}
	}
	t.Fatalf("no model %s", name)
	return razer.Model{}
}

func TestReadOnlyMouseRefusesApplyAndPreview(t *testing.T) {
	m := withMouse(t, modelNamed(t, "Viper"))
	if !strings.Contains(m.render(), "write whitelist") {
		t.Fatal("no warning on screen")
	}
	for tab := range m.tabs {
		m.cur, m.msg = tab, ""
		if !m.tabs[tab].mouse {
			continue
		}
		if cmd := m.applyTab(); cmd != nil || !m.msgErr {
			t.Errorf("%s: apply went through", m.tabs[tab].name)
		}
	}
	m.cur, m.cursor = 0, 0
	m.logo.effect.setChoice(string(razer.EffectStatic))
	m.cursor = 1
	m.updateKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.picker != nil {
		t.Fatal("the live colour picker opened")
	}
}

func TestReadOnlyMouseDoesNotBlockTheDock(t *testing.T) {
	m := withMouse(t, modelNamed(t, "Viper"))
	m.cur = 1
	if m.applyTab() == nil {
		t.Fatal("dock apply refused")
	}
}

func TestFieldsFollowTheModel(t *testing.T) {
	m := withMouse(t, modelNamed(t, "DeathAdder V2"))
	if m.stageCount.visible() || m.idle.visible() || m.lowBattery.visible() {
		t.Fatal("showing settings this mouse does not have")
	}
	m = withMouse(t, modelNamed(t, "Viper Ultimate"))
	if !m.stageCount.visible() || !m.idle.visible() {
		t.Fatal("hiding settings this mouse has")
	}
}

func TestUnknownRazerDeviceIsReported(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.found.Unknown = []uint16{0x00ff}
	if !strings.Contains(m.render(), "1532:00ff") {
		t.Fatal("no warning on screen")
	}
}
