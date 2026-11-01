package via

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// writeSafe is the whitelist of keyboards tested with this program. Only code
// can add to it: a definition file describes a keyboard, it cannot grant writes.
// Every other keyboard is read-only.
var writeSafe = map[[2]uint16]bool{
	{0x320f, 0x5055}: true, // Wobkey Crush 80, wired
	{0x320f, 0x5088}: true, // Wobkey Crush 80, 2.4G receiver
}

// builtin holds definitions that need no file.
var builtin = []*Definition{&Crush80}

// Key is one physical key. X and W are in key units.
type Key struct {
	X, W     float64
	Row, Col int // switch matrix position
	Stock    string
}

// LightingMenu addresses the backlight through VIA's custom menu commands. An
// ID of 0 means the keyboard lacks that setting.
type LightingMenu struct {
	Channel, Brightness, Effect, Speed, Color byte
}

// Definition describes a keyboard, which VIA's protocol does not do by itself.
type Definition struct {
	Name          string
	Vendor        uint16
	Products      []uint16
	Rows, Cols    int
	Keys          [][]Key
	Custom        []string // short names of the custom keycodes, in order
	Menu          *LightingMenu
	Effects       []string // indexed by effect value; may be empty
	MaxBrightness int
	MaxSpeed      int
	RawInterface  int  // USB interface of raw HID; 0 means unknown until connected
	WriteSafe     bool // set by Lookup from the whitelist, never by a file
}

// Width is the board width in key units.
func (d *Definition) Width() float64 {
	width := 0.0
	for _, row := range d.Keys {
		for _, k := range row {
			width = max(width, k.X+k.W)
		}
	}
	return width
}

var slugPart = regexp.MustCompile(`[a-z0-9]+`)

// Slug names the keyboard's folder. It can't leave that folder's parent.
func (d *Definition) Slug() string {
	parts := slugPart.FindAllString(strings.ToLower(d.Name), -1)
	if len(parts) == 0 {
		return "keyboard"
	}
	return strings.Join(parts, "-")
}

// Matches tells whether the definition describes a USB device.
func (d *Definition) Matches(vendor, product uint16) bool {
	if d.Vendor != vendor {
		return false
	}
	for _, p := range d.Products {
		if p == product {
			return true
		}
	}
	return false
}

// Builtin returns the definitions that need no file.
func Builtin() []*Definition { return builtin }

// Lookup finds the definition of a USB device, preferring built-in ones. It
// returns a copy whose WriteSafe comes from the whitelist, or nil.
func Lookup(vendor, product uint16, imported []*Definition) *Definition {
	for _, list := range [][]*Definition{builtin, imported} {
		for _, d := range list {
			if d.Matches(vendor, product) {
				c := *d
				c.WriteSafe = writeSafe[[2]uint16{vendor, product}]
				return &c
			}
		}
	}
	return nil
}

// LoadDefinitions reads every *.json in dir. Files that fail are reported, and
// the rest still load.
func LoadDefinitions(dir string) ([]*Definition, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var defs []*Definition
	var errs []error
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err == nil {
			var d *Definition
			if d, err = ParseDefinition(data); err == nil {
				defs = append(defs, d)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
		}
	}
	return defs, errors.Join(errs...)
}

// ImportDefinition validates a VIA definition file and stores it in dir. Built-in
// keyboards keep their own, which may know more than the file.
func ImportDefinition(path, dir string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	def, err := ParseDefinition(data)
	if err != nil {
		return nil, err
	}
	if known := Lookup(def.Vendor, def.Products[0], nil); known != nil {
		return nil, fmt.Errorf("%s is built in and needs no definition", known.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return def, os.WriteFile(filepath.Join(dir, def.Slug()+".json"), data, 0o644)
}

type rawDefinition struct {
	Name      string `json:"name"`
	VendorID  string `json:"vendorId"`
	ProductID string `json:"productId"`
	Matrix    struct{ Rows, Cols int }
	Layouts   struct {
		Keymap [][]json.RawMessage `json:"keymap"`
	} `json:"layouts"`
	CustomKeycodes []struct {
		Name      string `json:"name"`
		ShortName string `json:"shortName"`
	} `json:"customKeycodes"`
	Menus []json.RawMessage `json:"menus"`
}

// ParseDefinition reads a VIA definition, the JSON that VIA's Design tab loads.
func ParseDefinition(data []byte) (*Definition, error) {
	var r rawDefinition
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("not a VIA definition: %w", err)
	}
	vendor, err := parseID(r.VendorID)
	if err != nil {
		return nil, fmt.Errorf("vendorId: %w", err)
	}
	product, err := parseID(r.ProductID)
	if err != nil {
		return nil, fmt.Errorf("productId: %w", err)
	}
	if r.Name == "" {
		return nil, errors.New("definition has no name")
	}
	if r.Matrix.Rows <= 0 || r.Matrix.Cols <= 0 || r.Matrix.Rows > 64 || r.Matrix.Cols > 64 {
		return nil, fmt.Errorf("implausible matrix %dx%d", r.Matrix.Rows, r.Matrix.Cols)
	}
	d := &Definition{
		Name: r.Name, Vendor: vendor, Products: []uint16{product},
		Rows: r.Matrix.Rows, Cols: r.Matrix.Cols,
	}
	if d.Keys, err = parseLayout(r.Layouts.Keymap, d.Rows, d.Cols); err != nil {
		return nil, err
	}
	for _, c := range r.CustomKeycodes {
		name := c.ShortName
		if name == "" {
			name = c.Name
		}
		d.Custom = append(d.Custom, name)
	}
	for _, m := range r.Menus {
		walkMenu(m, d)
	}
	return d, nil
}

func parseID(s string) (uint16, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a USB ID", s)
	}
	return uint16(v), nil
}

// parseLayout reads VIA's keyboard-layout-editor format. Where a layout offers
// options (ISO or ANSI, split keys), it keeps the first choice of each. Some
// files, Wobkey's among them, place alternatives off the board without marking
// them as options: left of the board, over another key, or on a matrix position
// already used. Those are dropped too.
func parseLayout(rows [][]json.RawMessage, matrixRows, matrixCols int) ([][]Key, error) {
	var out [][]Key
	seen := map[[2]int]bool{}
	for _, row := range rows {
		var keys []Key
		x, w, decal, end := 0.0, 1.0, false, 0.0
		for _, item := range row {
			var props struct {
				X, W, R float64
				RX      float64 `json:"rx"`
				RY      float64 `json:"ry"`
				Decal   bool    `json:"d"`
			}
			if err := json.Unmarshal(item, &props); err == nil {
				if props.R != 0 || props.RX != 0 || props.RY != 0 {
					return nil, errors.New("rotated layouts are not supported")
				}
				x += props.X
				if props.W != 0 {
					w = props.W
				}
				decal = props.Decal
				continue
			}
			var label string
			if err := json.Unmarshal(item, &label); err != nil {
				return nil, fmt.Errorf("layout entry %s: %w", item, err)
			}
			if !decal {
				k, keep, err := parseKeyLabel(label, x, w, matrixRows, matrixCols)
				if err != nil {
					return nil, err
				}
				pos := [2]int{k.Row, k.Col}
				if keep && k.X >= end && !seen[pos] {
					keys = append(keys, k)
					seen[pos] = true
					end = k.X + k.W
				}
			}
			x += w
			w, decal = 1, false
		}
		if len(keys) > 0 {
			out = append(out, keys)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("definition has no keys")
	}
	return out, nil
}

func parseKeyLabel(label string, x, w float64, matrixRows, matrixCols int) (Key, bool, error) {
	parts := strings.Split(label, "\n")
	if len(parts) > 3 && parts[3] != "" {
		if option := strings.Split(parts[3], ","); len(option) == 2 && option[1] != "0" {
			return Key{}, false, nil
		}
	}
	pos := strings.Split(parts[0], ",")
	if len(pos) != 2 {
		return Key{}, false, nil // an encoder or a label without a matrix position
	}
	row, err1 := strconv.Atoi(strings.TrimSpace(pos[0]))
	col, err2 := strconv.Atoi(strings.TrimSpace(pos[1]))
	if err1 != nil || err2 != nil {
		return Key{}, false, nil
	}
	if row < 0 || col < 0 || row >= matrixRows || col >= matrixCols {
		return Key{}, false, fmt.Errorf("key %d,%d is outside the %dx%d matrix", row, col, matrixRows, matrixCols)
	}
	return Key{X: x, W: w, Row: row, Col: col, Stock: parts[0]}, true, nil
}

// presets are the menus VIA defines by name instead of listing their content.
var presets = map[string]LightingMenu{
	"qmk_backlight":  {Channel: 1, Brightness: 1, Effect: 2},
	"qmk_rgblight":   {Channel: 2, Brightness: 1, Effect: 2, Speed: 3, Color: 4},
	"qmk_rgb_matrix": {Channel: 3, Brightness: 1, Effect: 2, Speed: 3, Color: 4},
}

// walkMenu finds the backlight settings anywhere in a VIA menu tree.
func walkMenu(raw json.RawMessage, d *Definition) {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		if p, ok := presets[name]; ok && d.Menu == nil {
			d.Menu = &p
			d.MaxBrightness, d.MaxSpeed = 255, 255
		}
		return
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, item := range list {
			walkMenu(item, d)
		}
		return
	}
	var node struct {
		Content []json.RawMessage `json:"content"`
		Options []json.RawMessage `json:"options"`
	}
	if json.Unmarshal(raw, &node) != nil {
		return
	}
	if len(node.Content) == 3 {
		var id string
		var channel, value byte
		if json.Unmarshal(node.Content[0], &id) == nil &&
			json.Unmarshal(node.Content[1], &channel) == nil &&
			json.Unmarshal(node.Content[2], &value) == nil {
			setting(id, channel, value, node.Options, d)
			return
		}
	}
	for _, item := range node.Content {
		walkMenu(item, d)
	}
}

func setting(id string, channel, value byte, options []json.RawMessage, d *Definition) {
	if !strings.HasPrefix(id, "id_qmk_") {
		return
	}
	if d.Menu == nil {
		d.Menu = &LightingMenu{Channel: channel}
	}
	if d.Menu.Channel != channel {
		return // a second lighting zone; the first one is what is shown
	}
	switch {
	case strings.HasSuffix(id, "_effect_speed"):
		d.Menu.Speed = value
		d.MaxSpeed = rangeMax(options)
	case strings.HasSuffix(id, "_brightness"):
		d.Menu.Brightness = value
		d.MaxBrightness = rangeMax(options)
	case strings.HasSuffix(id, "_effect"):
		d.Menu.Effect = value
		d.Effects = effectNames(options)
	case strings.HasSuffix(id, "_color"):
		d.Menu.Color = value
	}
}

func rangeMax(options []json.RawMessage) int {
	top := 255
	if len(options) == 2 {
		_ = json.Unmarshal(options[1], &top)
	}
	return top
}

// effectNames keeps the names only if they can be indexed by value.
func effectNames(options []json.RawMessage) []string {
	var names []string
	for i, option := range options {
		var pair []json.RawMessage
		if json.Unmarshal(option, &pair) != nil || len(pair) != 2 {
			return nil
		}
		var name string
		var value int
		if json.Unmarshal(pair[0], &name) != nil || json.Unmarshal(pair[1], &value) != nil || value != i {
			return nil
		}
		names = append(names, name)
	}
	return names
}
