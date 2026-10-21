package desktop

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// InstallMacMenu installs the macOS menu bar. It is required, not
// decoration: WKWebView only receives ⌘C/⌘V/⌘X/⌘A through an Edit menu's
// roles, and ⌘Q/⌘H/⌘M come from the app and Window menus. Linux and
// Windows get none: the window is frameless or keyboard and command-palette
// driven there, so a menu bar would only add chrome. The menu API compiles
// on every OS, so this is a runtime check rather than a build tag.
func InstallMacMenu(a *application.App) {
	if runtime.GOOS != "darwin" {
		return
	}
	bar := application.NewMenu()
	am := bar.AddSubmenu("Quarro") // the application menu, named after the app
	am.AddRole(application.About)
	am.AddSeparator()
	// The frontend's own Ctrl/⌘+, handler never sees the keystroke while
	// this accelerator owns it, so the shortcut fires once either way.
	am.Add("Settings…").
		SetAccelerator("CmdOrCtrl+,").
		OnClick(func(*application.Context) { a.Event.Emit(openSettings) })
	am.AddSeparator()
	am.AddRole(application.ServicesMenu)
	am.AddSeparator()
	am.AddRole(application.Hide)
	am.AddRole(application.HideOthers)
	am.AddRole(application.UnHide)
	am.AddSeparator()
	am.AddRole(application.Quit)
	bar.AddRole(application.EditMenu)
	bar.AddRole(application.WindowMenu)
	a.Menu.Set(bar)
}
