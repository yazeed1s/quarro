// Package stage defines build modes shared by the embedded UI and backend.
package stage

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

func Read(assets fs.FS) (string, error) {
	data, err := fs.ReadFile(assets, "frontend/dist/app-stage.json")
	if err != nil {
		return "", fmt.Errorf(
			"build frontend before starting Quarro: %w",
			err,
		)
	}
	var mode string
	if err = json.Unmarshal(data, &mode); err != nil {
		return "", err
	}
	switch mode {
	case "preview", "test", "prod":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid application stage %q", mode)
	}
}

// AppDir is the single directory (under the OS config and data dirs) where
// Quarro persists its files for every build stage.
const AppDir = "quarro"

// Suffix distinguishes non-prod build stages' filenames within AppDir, so
// preview/test runs get their own profiles.json and demo.db instead of
// sharing (or clobbering) prod's. Prod uses no suffix.
func Suffix(mode string) string {
	if mode == "prod" {
		return ""
	}
	return "-" + mode
}
