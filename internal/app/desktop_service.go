package app

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

// DesktopWindow is the slice of the live Wails window DesktopService needs,
// injected for the same reason as SaveFileDialog (see dialogs.go).
type DesktopWindow interface {
	SetFrameless(frameless bool)
	SetBackground(r, g, b uint8)
	// SetAppIcon fails where the icon lives outside the app, e.g. a Linux
	// launcher entry that is not installed.
	SetAppIcon(icon windowprefs.AppIcon) error
}

// ClipboardWriter writes text to the OS clipboard and reports whether it
// did; Wails' server mode has no clipboard and reports false.
type ClipboardWriter interface {
	SetText(text string) bool
}

// DesktopChrome tells the frontend how the window is decorated.
type DesktopChrome struct {
	Platform string `json:"platform"`
	TitleBar string `json:"titleBar"`
	// Configurable is true where the title bar choice exists (Linux).
	Configurable bool `json:"configurable"`
	// Frameless means the frontend must draw the drag region and window
	// controls itself.
	Frameless bool `json:"frameless"`
	// TrafficLights means macOS draws its window controls over the leading
	// edge of the session bar, which stays the drag region but must leave
	// room for them.
	TrafficLights bool   `json:"trafficLights"`
	AppIcon       string `json:"appIcon"`
	// AppIconConfigurable is true where the running app's icon can be
	// swapped: the Dock (macOS), taskbar (Windows) or launcher entry (Linux);
	// see windowprefs.AppIconSwappable.
	AppIconConfigurable bool `json:"appIconConfigurable"`
}

type DesktopService struct {
	mu        sync.Mutex
	store     *windowprefs.Store
	prefs     windowprefs.Prefs
	goos      string
	window    DesktopWindow
	clipboard ClipboardWriter
}

// NewDesktopService takes the prefs main.go already loaded to create the
// window, so the service and the window start from the same values.
func NewDesktopService(
	store *windowprefs.Store,
	prefs windowprefs.Prefs,
	goos string,
	window DesktopWindow,
	clipboard ClipboardWriter,
) *DesktopService {
	return &DesktopService{
		store:     store,
		prefs:     prefs,
		goos:      goos,
		window:    window,
		clipboard: clipboard,
	}
}

func (s *DesktopService) Chrome() DesktopChrome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chrome()
}

func (s *DesktopService) chrome() DesktopChrome {
	return DesktopChrome{
		Platform:            s.goos,
		TitleBar:            string(s.prefs.TitleBar),
		Configurable:        s.goos == "linux",
		Frameless:           windowprefs.Frameless(s.goos, s.prefs.TitleBar),
		TrafficLights:       s.goos == "darwin",
		AppIcon:             string(s.prefs.AppIcon),
		AppIconConfigurable: windowprefs.AppIconSwappable(s.goos),
	}
}

// SetTitleBar saves the choice before applying it live, so even if a
// compositor mishandles the live switch the next launch starts correctly.
func (s *DesktopService) SetTitleBar(titleBar string) (DesktopChrome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.goos != "linux" {
		return s.chrome(), fmt.Errorf("the title bar can only be changed on Linux")
	}
	next := s.prefs
	next.TitleBar = windowprefs.TitleBar(titleBar)
	if next.TitleBar != windowprefs.TitleBarSystem &&
		next.TitleBar != windowprefs.TitleBarCustom {
		return s.chrome(), fmt.Errorf("unknown title bar %q", titleBar)
	}
	if next == s.prefs {
		return s.chrome(), nil
	}
	if err := s.store.Save(next); err != nil {
		return s.chrome(), fmt.Errorf("save window preferences: %w", err)
	}
	s.prefs = next
	s.window.SetFrameless(windowprefs.Frameless(s.goos, next.TitleBar))
	return s.chrome(), nil
}

// SetAppIcon applies the choice before saving it, unlike SetTitleBar: on
// Linux applying it is what can fail (the launcher entry or icon is missing),
// and a choice that never took effect should not be remembered.
func (s *DesktopService) SetAppIcon(icon string) (DesktopChrome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !windowprefs.AppIconSwappable(s.goos) {
		return s.chrome(), fmt.Errorf("the app icon cannot be changed on %s", s.goos)
	}
	next := s.prefs
	next.AppIcon = windowprefs.AppIcon(icon)
	if next.AppIcon != windowprefs.AppIconDark &&
		next.AppIcon != windowprefs.AppIconLight {
		return s.chrome(), fmt.Errorf("unknown app icon %q", icon)
	}
	if next == s.prefs {
		return s.chrome(), nil
	}
	if err := s.window.SetAppIcon(next.AppIcon); err != nil {
		return s.chrome(), err
	}
	if err := s.store.Save(next); err != nil {
		// Best effort: put the icon back to match what is saved.
		_ = s.window.SetAppIcon(s.prefs.AppIcon)
		return s.chrome(), fmt.Errorf("save window preferences: %w", err)
	}
	s.prefs = next
	return s.chrome(), nil
}

// SetBackground records the theme's resolved --bg ("#rrggbb") so the next
// launch paints it before the frontend's first frame, and applies it now so
// areas the webview has not painted yet (e.g. during a resize) match.
func (s *DesktopService) SetBackground(hex string) error {
	hex = strings.ToLower(strings.TrimSpace(hex))
	r, g, b, ok := windowprefs.ParseHex(hex)
	if !ok {
		return fmt.Errorf("invalid background colour %q", hex)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if hex == s.prefs.Background {
		return nil
	}
	next := s.prefs
	next.Background = hex
	if err := s.store.Save(next); err != nil {
		return fmt.Errorf("save window preferences: %w", err)
	}
	s.prefs = next
	s.window.SetBackground(r, g, b)
	return nil
}

// CopyText writes to the OS clipboard. False means the frontend should fall
// back to the browser clipboard API (server mode, browser preview).
func (s *DesktopService) CopyText(text string) bool {
	return s.clipboard.SetText(text)
}
