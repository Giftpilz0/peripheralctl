// peripheralctl configures Razer mice and VIA keyboards from Linux and keeps the
// mouse lighting preset applied across reconnects. Models off the write whitelist
// are shown read-only.
//
// It only sends runtime configuration commands; it cannot flash firmware.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/giftpilz0/peripheralctl/internal/app"
	"github.com/giftpilz0/peripheralctl/internal/config"
	"github.com/giftpilz0/peripheralctl/internal/daemon"
	"github.com/giftpilz0/peripheralctl/internal/setup"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

const usage = `usage: peripheralctl [command]

commands:
  (none)        terminal UI for the mouse and keyboard (F1/F2 switch)
  import FILE   add a keyboard from its VIA JSON definition (stays read-only)
  install       install this binary, its udev rule and the user service
                (asks for sudo once for the udev rule; safe to re-run to upgrade)
  uninstall     remove all of that again; add --purge to delete the settings too
  daemon        keep the saved preset applied (run by the user service)
`

func main() {
	var err error
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		err = app.Run()
	case args[0] == "import" && len(args) == 2:
		err = importKeyboard(args[1])
	case args[0] == "install":
		err = setup.Install()
	case args[0] == "uninstall":
		err = setup.Uninstall(len(args) > 1 && args[1] == "--purge")
	case args[0] == "daemon":
		log.SetFlags(0) // journald adds timestamps
		err = daemon.Run()
	case args[0] == "-h", args[0] == "--help", args[0] == "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "peripheralctl:", err)
		os.Exit(1)
	}
}

func importKeyboard(path string) error {
	def, err := via.ImportDefinition(path, config.KeyboardsDir())
	if err != nil {
		return err
	}
	fmt.Printf("imported %s (%04x:%04x). It stays read-only: only models on the whitelist in the code get writes.\n",
		def.Name, def.Vendor, def.Products[0])
	fmt.Println("Connect it and run 'peripheralctl install' to give yourself access to it.")
	return nil
}
