//go:build !darwin && !windows && !linux

package desktop

import (
	"fmt"
	"runtime"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

func Icon(windowprefs.AppIcon) []byte {
	return nil
}

func setAppIcon(windowprefs.AppIcon) error {
	return fmt.Errorf("the app icon cannot be changed on %s", runtime.GOOS)
}
