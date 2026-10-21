package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

type fakeWindow struct {
	frameless  []bool
	background []string
	icons      []windowprefs.AppIcon
	iconErr    error
}

func (w *fakeWindow) SetFrameless(frameless bool) {
	w.frameless = append(w.frameless, frameless)
}

func (w *fakeWindow) SetBackground(r, g, b uint8) {
	w.background = append(w.background, string([]byte{r, g, b}))
}

func (w *fakeWindow) SetAppIcon(icon windowprefs.AppIcon) error {
	if w.iconErr != nil {
		return w.iconErr
	}
	w.icons = append(w.icons, icon)
	return nil
}

type fakeClipboard struct {
	ok   bool
	text string
}

func (c *fakeClipboard) SetText(text string) bool {
	c.text = text
	return c.ok
}

func newDesktop(t *testing.T, goos string) (*DesktopService, *fakeWindow, *windowprefs.Store) {
	t.Helper()
	store := windowprefs.NewStoreAt(filepath.Join(t.TempDir(), "window.json"))
	window := &fakeWindow{}
	return NewDesktopService(store, windowprefs.Default(), goos, window, &fakeClipboard{}), window, store
}

func TestDesktopChrome(t *testing.T) {
	linux, _, _ := newDesktop(t, "linux")
	if got := linux.Chrome(); !got.Configurable || got.Frameless || got.TitleBar != "system" {
		t.Fatalf("linux default chrome = %+v", got)
	}
	if got := linux.Chrome(); got.TrafficLights {
		t.Fatalf("linux chrome reports traffic lights: %+v", got)
	}
	mac, _, _ := newDesktop(t, "darwin")
	if got := mac.Chrome(); got.Configurable || got.Frameless || !got.TrafficLights {
		t.Fatalf("darwin chrome = %+v", got)
	}
	windows, _, _ := newDesktop(t, "windows")
	if got := windows.Chrome(); got.Configurable || !got.Frameless || got.TrafficLights {
		t.Fatalf("windows chrome = %+v", got)
	}
}

func TestSetTitleBarSavesThenApplies(t *testing.T) {
	svc, window, store := newDesktop(t, "linux")
	got, err := svc.SetTitleBar("custom")
	if err != nil || !got.Frameless || got.TitleBar != "custom" {
		t.Fatalf("SetTitleBar = %+v, %v", got, err)
	}
	if len(window.frameless) != 1 || !window.frameless[0] {
		t.Fatalf("SetFrameless calls = %v", window.frameless)
	}
	if saved, _ := store.Load(); saved.TitleBar != windowprefs.TitleBarCustom {
		t.Fatalf("saved = %+v", saved)
	}
	// Re-selecting the current value touches neither the file nor the window.
	if _, err := svc.SetTitleBar("custom"); err != nil || len(window.frameless) != 1 {
		t.Fatalf("no-op SetTitleBar: %v, calls %v", err, window.frameless)
	}
}

func TestSetTitleBarRejects(t *testing.T) {
	svc, window, _ := newDesktop(t, "linux")
	if _, err := svc.SetTitleBar("fancy"); err == nil {
		t.Fatal("unknown value accepted")
	}
	mac, macWindow, _ := newDesktop(t, "darwin")
	if _, err := mac.SetTitleBar("system"); err == nil {
		t.Fatal("title bar change accepted on darwin")
	}
	if len(window.frameless)+len(macWindow.frameless) != 0 {
		t.Fatal("rejected change reached the window")
	}
}

func TestSetTitleBarSaveFailureLeavesWindowAlone(t *testing.T) {
	// A file where the store expects its parent directory makes Save fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	window := &fakeWindow{}
	svc := NewDesktopService(
		windowprefs.NewStoreAt(filepath.Join(blocker, "window.json")),
		windowprefs.Default(), "linux", window, &fakeClipboard{},
	)
	got, err := svc.SetTitleBar("custom")
	if err == nil || got.TitleBar != "system" || len(window.frameless) != 0 {
		t.Fatalf("got %+v, %v, calls %v", got, err, window.frameless)
	}
}

func TestSetAppIcon(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		svc, window, store := newDesktop(t, goos)
		if got := svc.Chrome(); !got.AppIconConfigurable || got.AppIcon != "dark" {
			t.Fatalf("%s default chrome = %+v", goos, got)
		}
		got, err := svc.SetAppIcon("light")
		if err != nil || got.AppIcon != "light" {
			t.Fatalf("%s SetAppIcon = %+v, %v", goos, got, err)
		}
		if len(window.icons) != 1 || window.icons[0] != windowprefs.AppIconLight {
			t.Fatalf("%s SetAppIcon calls = %v", goos, window.icons)
		}
		if saved, _ := store.Load(); saved.AppIcon != windowprefs.AppIconLight {
			t.Fatalf("%s saved = %+v", goos, saved)
		}
		if _, err := svc.SetAppIcon("light"); err != nil || len(window.icons) != 1 {
			t.Fatalf("%s no-op SetAppIcon: %v, calls %v", goos, err, window.icons)
		}
		if _, err := svc.SetAppIcon("neon"); err == nil || len(window.icons) != 1 {
			t.Fatalf("%s unknown icon: %v, calls %v", goos, err, window.icons)
		}
	}
}

func TestSetAppIconFailureIsNotSaved(t *testing.T) {
	svc, window, store := newDesktop(t, "linux")
	window.iconErr = errors.New("quarro.desktop is not installed")
	got, err := svc.SetAppIcon("light")
	if err == nil || got.AppIcon != "dark" {
		t.Fatalf("SetAppIcon = %+v, %v", got, err)
	}
	if saved, _ := store.Load(); saved.AppIcon != windowprefs.AppIconDark {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestSetAppIconRejectedWhereUnsupported(t *testing.T) {
	svc, window, _ := newDesktop(t, "freebsd")
	if got := svc.Chrome(); got.AppIconConfigurable {
		t.Fatalf("freebsd chrome = %+v", got)
	}
	if _, err := svc.SetAppIcon("light"); err == nil || len(window.icons) != 0 {
		t.Fatalf("freebsd SetAppIcon: %v, calls %v", err, window.icons)
	}
}

func TestSetBackground(t *testing.T) {
	svc, window, store := newDesktop(t, "linux")
	if err := svc.SetBackground(" #F4F5F5 "); err != nil {
		t.Fatal(err)
	}
	if saved, _ := store.Load(); saved.Background != "#f4f5f5" {
		t.Fatalf("saved = %+v", saved)
	}
	if len(window.background) != 1 || window.background[0] != "\xf4\xf5\xf5" {
		t.Fatalf("SetBackground calls = %q", window.background)
	}
	if err := svc.SetBackground("#f4f5f5"); err != nil || len(window.background) != 1 {
		t.Fatalf("unchanged colour re-applied: %v", err)
	}
	if err := svc.SetBackground("rgb(1, 2, 3)"); err == nil {
		t.Fatal("non-hex colour accepted")
	}
}

func TestCopyText(t *testing.T) {
	clipboard := &fakeClipboard{ok: true}
	svc := NewDesktopService(nil, windowprefs.Default(), "linux", &fakeWindow{}, clipboard)
	if !svc.CopyText("hello") || clipboard.text != "hello" {
		t.Fatalf("CopyText wrote %q", clipboard.text)
	}
	clipboard.ok = false
	if svc.CopyText("x") {
		t.Fatal("CopyText reported success when the clipboard did not")
	}
}
