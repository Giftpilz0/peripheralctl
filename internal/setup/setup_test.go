package setup

import (
	"strings"
	"testing"

	"github.com/giftpilz0/peripheralctl/internal/razer"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

func TestUnitMatchesBinPath(t *testing.T) {
	if !strings.Contains(string(unitFile), "ExecStart=%h/.local/bin/peripheralctl daemon") {
		t.Fatal("unit ExecStart does not match binPath")
	}
}

func TestRuleFile(t *testing.T) {
	var keyboards []keyboard
	for _, p := range via.Crush80.Products {
		keyboards = append(keyboards, keyboard{via.Crush80.Vendor, p, via.Crush80.RawInterface})
	}
	keyboards = append(keyboards, keyboard{0xfeed, 0x0001, 2})
	rule := string(ruleFile(razer.ProductIDs(), keyboards))
	for _, want := range []string{
		`ATTRS{idVendor}=="1532"`,
		"007c|007d|007e",
		`KERNELS=="0003:320F:5055.*|0003:320F:5088.*", GOTO="peripheralctl_if01"`,
		`ATTRS{bInterfaceNumber}=="01", TAG+="uaccess"`,
		`KERNELS=="0003:FEED:0001.*", GOTO="peripheralctl_if02"`,
		`ATTRS{bInterfaceNumber}=="02", TAG+="uaccess"`,
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("missing %s in:\n%s", want, rule)
		}
	}
	// A keyboard rule without an interface would expose keystrokes.
	for line := range strings.SplitSeq(rule, "\n") {
		if strings.Contains(line, "uaccess") && !strings.Contains(line, "bInterfaceNumber") && !strings.Contains(line, "1532") {
			t.Errorf("unlimited rule: %s", line)
		}
	}
}
