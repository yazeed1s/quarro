// Package desktop is Quarro's Wails glue: the main window, the macOS menu,
// the app icon, and the live implementations of internal/app's desktop
// interfaces. It is kept out of internal/app so that package can build and
// test without pulling in Linux cgo or GTK dependencies.
package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

const (
	// mainWindow names the main window so Window can find it, since
	// DesktopService is built before the window exists.
	mainWindow = "main"
	// openSettings asks the frontend to open Settings (macOS menu).
	openSettings = "quarro:open-settings"
)

// Dialogs implements internal/app's dialog interfaces.
type Dialogs struct{}

// SaveFile takes the default file name n and one filter's label and pattern.
func (Dialogs) SaveFile(
	n, lbl, pat string,
) (string, error) {
	return application.Get().Dialog.SaveFile().
		SetFilename(n).
		AddFilter(lbl, pat).
		PromptForSingleSelection()
}

// OpenFile takes the dialog title t and one filter's label and pattern.
func (Dialogs) OpenFile(
	t, lbl, pat string,
) (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle(t).
		CanChooseFiles(true).
		AddFilter(lbl, pat).
		PromptForSingleSelection()
}

// Window implements internal/app.DesktopWindow against the live main
// window, looked up lazily because DesktopService is created before it.
type Window struct{}

func (Window) SetFrameless(on bool) {
	if w, ok := application.Get().Window.GetByName(mainWindow); ok {
		w.SetFrameless(on)
	}
}

func (Window) SetBackground(r, g, b uint8) {
	if w, ok := application.Get().Window.GetByName(mainWindow); ok {
		w.SetBackgroundColour(application.NewRGB(r, g, b))
	}
}

// SetAppIcon applies the App icon choice (see setAppIcon in icon_*.go and
// windowprefs.AppIconSwappable).
func (Window) SetAppIcon(i windowprefs.AppIcon) error {
	return setAppIcon(i)
}

// Clipboard implements internal/app.ClipboardWriter.
type Clipboard struct{}

func (Clipboard) SetText(s string) bool {
	return application.Get().Clipboard.SetText(s)
}
