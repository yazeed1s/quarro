//go:build windows

package desktop

import (
	_ "embed"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

// Multi-size .ico files (16–256px, written by brand/generate_icons.py), so
// Windows picks the frame drawn for each icon size and DPI instead of
// scaling one bitmap.
var (
	//go:embed icons/dark.ico
	darkIcon []byte
	//go:embed icons/light.ico
	lightIcon []byte
)

// icons are the big and small HICONs setAppIcon last gave the window,
// destroyed once replaced.
var icons [2]w32.HICON

// Icon is Options.Icon, which Wails only falls back to when the .exe has no
// embedded icon.
func Icon(i windowprefs.AppIcon) []byte {
	if i == windowprefs.AppIconLight {
		return lightIcon
	}
	return darkIcon
}

// setAppIcon swaps the window's taskbar, Alt+Tab and title-bar icon.
// Explorer, the Start menu and pinned shortcuts show the icon embedded in the
// .exe at build time (see build/windows), which a running app cannot change.
func setAppIcon(i windowprefs.AppIcon) error {
	w, ok := application.Get().Window.GetByName(mainWindow)
	if !ok {
		return nil
	}
	hwnd := w32.HWND(w.NativeWindow())
	if hwnd == 0 {
		return nil
	}
	data := Icon(i)
	big, err := w32.CreateLargeHIconFromImage(data)
	if err != nil {
		return fmt.Errorf("load the %s icon: %w", i, err)
	}
	small, err := w32.CreateSmallHIconFromImage(data)
	if err != nil {
		w32.DestroyIcon(big)
		return fmt.Errorf("load the %s icon: %w", i, err)
	}
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, uintptr(big))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, uintptr(small))
	for _, old := range icons {
		if old != 0 {
			w32.DestroyIcon(old)
		}
	}
	icons = [2]w32.HICON{big, small}
	return nil
}
