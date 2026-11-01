package kbdtui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/giftpilz0/peripheralctl/internal/config"
	"github.com/giftpilz0/peripheralctl/internal/via"
)

// autoProfile holds the keyboard's state from just before the last write.
const autoProfile = "before-last-write"

const profileVersion = 1

// profile is the file format. It holds the keymap and backlight, the two things
// VIA can read back.
type profile struct {
	Version int        `json:"version"`
	SavedAt time.Time  `json:"saved_at"`
	State   *via.State `json:"state"`
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`)

func checkName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use letters, digits, . _ and -", name)
	}
	return nil
}

// profileDir keeps each keyboard's profiles apart: keycodes of one keyboard mean
// nothing on another of the same size.
func profileDir(def *via.Definition) string {
	return filepath.Join(filepath.Dir(config.Path()), "keyboard", def.Slug())
}

func profilePath(dir, name string) string { return filepath.Join(dir, name+".json") }

func saveProfile(dir, name string, s *via.State) error {
	if err := checkName(name); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile{profileVersion, time.Now().UTC(), s}, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := profilePath(dir, name) + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, profilePath(dir, name))
}

func loadProfile(dir, name string) (*via.State, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(profilePath(dir, name))
	if err != nil {
		return nil, err
	}
	var p profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("profile %s: %w", name, err)
	}
	if p.Version != profileVersion || p.State == nil {
		return nil, fmt.Errorf("profile %s: unsupported format", name)
	}
	return p.State, nil
}

func listProfiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func deleteProfile(dir, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	return os.Remove(profilePath(dir, name))
}

// backend is the keyboard as the TUI and the commands see it.
type backend interface {
	Load() (*via.State, error)
	// Apply makes the keyboard match want and returns its new state. A preview
	// leaves the keyboard reporting its colour, so saved names the colour it had
	// before, which makes the preview count as a change that needs saving.
	Apply(want *via.State, saved *via.Color) (*via.State, via.Changes, error)
	// Preview shows a colour without the save command.
	Preview(c via.Color) error
}

type device struct {
	path string
	def  *via.Definition
}

func newDevice(path string, def *via.Definition) backend { return device{path, def} }

// deviceLock keeps a preview and an apply from mixing their replies.
var deviceLock sync.Mutex

func (d device) Preview(c via.Color) error {
	deviceLock.Lock()
	defer deviceLock.Unlock()
	dev, err := via.Open(d.path, d.def)
	if err != nil {
		return err
	}
	defer func() { _ = dev.Close() }()
	return dev.PreviewColor(c)
}

func (d device) Load() (*via.State, error) {
	deviceLock.Lock()
	defer deviceLock.Unlock()
	dev, err := via.Open(d.path, d.def)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dev.Close() }()
	return dev.State(d.def.Rows, d.def.Cols)
}

func (d device) Apply(want *via.State, saved *via.Color) (*via.State, via.Changes, error) {
	deviceLock.Lock()
	defer deviceLock.Unlock()
	dev, err := via.Open(d.path, d.def)
	if err != nil {
		return nil, via.Changes{}, err
	}
	defer func() { _ = dev.Close() }()
	cur, err := dev.State(d.def.Rows, d.def.Cols)
	if err != nil {
		return nil, via.Changes{}, err
	}
	if err := cur.Compatible(want); err != nil {
		return cur, via.Changes{}, err
	}
	if saved != nil {
		cur.Lighting.Hue, cur.Lighting.Saturation = saved.Hue, saved.Saturation
	}
	changes := cur.Diff(want)
	if changes.None() {
		return cur, changes, nil
	}
	if err := saveProfile(profileDir(d.def), autoProfile, cur); err != nil {
		return cur, via.Changes{}, fmt.Errorf("saving a backup first: %w", err)
	}
	changes, err = dev.Apply(cur, want)
	now, readErr := dev.State(d.def.Rows, d.def.Cols)
	return now, changes, errors.Join(err, readErr)
}

// Found is a VIA keyboard seen in sysfs; its Path is empty when there is none.
type Found struct {
	Path            string
	Vendor, Product uint16
}

func (f Found) definition() *via.Definition {
	imported, _ := via.LoadDefinitions(config.KeyboardsDir())
	return via.Lookup(f.Vendor, f.Product, imported)
}

// FindKeyboard picks a connected VIA keyboard from sysfs: one on the write
// whitelist first, then one with a definition, then any.
func FindKeyboard() (Found, error) {
	all, err := via.Detect()
	if err != nil {
		return Found{}, err
	}
	imported, loadErr := via.LoadDefinitions(config.KeyboardsDir())
	var best Found
	bestRank := -1
	for _, d := range all {
		rank := 0
		if def := via.Lookup(d.Vendor, d.Product, imported); def != nil {
			rank = 1
			if def.WriteSafe {
				rank = 2
			}
		}
		if rank > bestRank {
			best, bestRank = Found{d.Path, d.Vendor, d.Product}, rank
		}
	}
	return best, loadErr
}
