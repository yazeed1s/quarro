// Package windowprefs persists the few window settings Go has to know before
// the webview exists, and so cannot read from the frontend's localStorage:
// whether Linux uses the window manager's decorations or Quarro's own title
// bar, the colour to paint before the frontend's first frame, and which app
// icon to show in the Dock or taskbar.
package windowprefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/adrg/xdg"
)

type TitleBar string

const (
	// TitleBarSystem leaves decorations to the window manager.
	TitleBarSystem TitleBar = "system"
	// TitleBarCustom makes the window frameless; the frontend draws the
	// drag region and window controls itself.
	TitleBarCustom TitleBar = "custom"
)

type AppIcon string

const (
	// AppIconDark is the dark tile, the icon the installers and bundles ship.
	AppIconDark  AppIcon = "dark"
	AppIconLight AppIcon = "light"
)

// DefaultBackground matches the default (dark) theme's --bg.
const DefaultBackground = "#0d0e0e"

type Prefs struct {
	TitleBar TitleBar `json:"titleBar"`
	// Background is the last resolved theme --bg as "#rrggbb".
	Background string  `json:"background"`
	AppIcon    AppIcon `json:"appIcon"`
}

func Default() Prefs {
	return Prefs{TitleBar: TitleBarSystem, Background: DefaultBackground, AppIcon: AppIconDark}
}

// Frameless reports whether the window should be created without native
// decorations, leaving the frontend to draw window controls. Only Linux
// offers the choice. Windows is always frameless. macOS never is: it keeps
// a native window whose hidden-inset title bar puts the traffic lights over
// the session bar (see main.go).
func Frameless(goos string, t TitleBar) bool {
	switch goos {
	case "linux":
		return t == TitleBarCustom
	case "darwin":
		return false
	default:
		return true
	}
}

// AppIconSwappable reports whether the app icon can follow the AppIcon
// choice. macOS swaps the running app's Dock icon and Windows its window and
// taskbar icon; the bundle and the .exe keep the shipped (dark) icon. GTK4
// has no icon API, so Linux instead points the user's quarro.desktop launcher
// entry at the chosen icon, which launchers, docks and the window all follow.
func AppIconSwappable(goos string) bool {
	return goos == "darwin" || goos == "windows" || goos == "linux"
}

// ParseHex parses "#rrggbb".
func ParseHex(s string) (r, g, b uint8, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}

// normalize replaces each invalid field with its default, so one bad value
// never discards the other.
func normalize(p Prefs) Prefs {
	d := Default()
	if p.TitleBar != TitleBarSystem && p.TitleBar != TitleBarCustom {
		p.TitleBar = d.TitleBar
	}
	if _, _, _, ok := ParseHex(p.Background); !ok {
		p.Background = d.Background
	}
	if p.AppIcon != AppIconDark && p.AppIcon != AppIconLight {
		p.AppIcon = d.AppIcon
	}
	return p
}

type Store struct {
	mu   sync.Mutex
	path string
}

// NewStoreForStage returns a Store at "window<suffix>.json" under the user's
// config directory, alongside connections' profiles<suffix>.json.
func NewStoreForStage(dir, suffix string) (*Store, error) {
	path, err := xdg.ConfigFile(filepath.Join(dir, "window"+suffix+".json"))
	if err != nil {
		return nil, fmt.Errorf("resolve window preferences path: %w", err)
	}
	return NewStoreAt(path), nil
}

func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

// Load always returns usable Prefs: a missing file yields the defaults with
// a nil error, and an unreadable or corrupt one yields the defaults with the
// error, so startup can log it and carry on.
func (s *Store) Load() (Prefs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	var p Prefs
	if err := json.Unmarshal(data, &p); err != nil {
		return Default(), err
	}
	return normalize(p), nil
}

func (s *Store) Save(p Prefs) error {
	if normalize(p) != p {
		return fmt.Errorf("invalid window preferences %+v", p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".window-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
