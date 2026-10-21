//go:build darwin

package desktop

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

// The 1024px masters (written by brand/generate_icons.py) match the largest
// entry in the bundle's icons.icns, so AppKit only ever scales them down.
var (
	//go:embed icons/dark.png
	darkIcon []byte
	//go:embed icons/light.png
	lightIcon []byte
)

// Icon is Options.Icon: the Dock icon from launch, and the About box's.
func Icon(i windowprefs.AppIcon) []byte {
	if i == windowprefs.AppIconLight {
		return lightIcon
	}
	return darkIcon
}

// setAppIcon swaps the running app's Dock icon. Finder, Launchpad and the
// Dock after quitting keep the bundle's icons.icns, which macOS reads from
// the signed bundle and an app cannot change.
func setAppIcon(i windowprefs.AppIcon) error {
	application.Get().SetIcon(Icon(i))
	return nil
}
