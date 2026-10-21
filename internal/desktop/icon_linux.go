package desktop

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

// launcherEntry is the desktop entry the packages and
// scripts/install-local.sh install; its Icon= is what launchers, docks and
// the window show.
const launcherEntry = "quarro.desktop"

// Icon is Options.Icon, which GTK4 ignores; nothing is embedded on Linux.
func Icon(windowprefs.AppIcon) []byte {
	return nil
}

func setAppIcon(i windowprefs.AppIcon) error {
	return setLauncherIcon(xdg.DataHome, xdg.DataDirs, i)
}

// setLauncherIcon points the user's quarro.desktop at the quarro-dark or
// quarro-light icon (installed beside "quarro" by the packages and
// scripts/install-local.sh). A system-wide entry is copied into home first,
// since a user entry with the same name overrides it. "quarro" is the shipped
// dark icon, so choosing dark leaves an entry that names it untouched, and a
// fresh install never gains a user copy.
func setLauncherIcon(home string, dirs []string, i windowprefs.AppIcon) error {
	name := "quarro-" + string(i)
	all := append([]string{home}, dirs...)
	if !iconInstalled(all, name) {
		return fmt.Errorf("the %s icon is not installed; reinstall Quarro to add it", i)
	}
	var src string
	for _, dir := range all {
		if p := filepath.Join(dir, "applications", launcherEntry); fileExists(p) {
			src = p
			break
		}
	}
	if src == "" {
		return fmt.Errorf("the launcher entry %s is not installed", launcherEntry)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	next, changed := withIcon(string(data), name, i == windowprefs.AppIconDark)
	if !changed {
		return nil
	}
	return writeAtomic(filepath.Join(home, "applications", launcherEntry), []byte(next))
}

// withIcon sets Icon= in the [Desktop Entry] group, leaving other groups
// (e.g. [Desktop Action ...]) alone. keepDefault treats an existing
// Icon=quarro as already correct.
func withIcon(entry, name string, keepDefault bool) (string, bool) {
	var out []string
	group, set, changed := "", false, false
	sc := bufio.NewScanner(strings.NewReader(entry))
	for sc.Scan() {
		line := sc.Text()
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "[") {
			if group == "[Desktop Entry]" && !set {
				out = append(out, "Icon="+name)
				set, changed = true, true
			}
			group = t
		} else if group == "[Desktop Entry]" && strings.HasPrefix(t, "Icon=") {
			set = true
			cur := strings.TrimSpace(strings.TrimPrefix(t, "Icon="))
			if cur != name && !(keepDefault && cur == "quarro") {
				line, changed = "Icon="+name, true
			}
		}
		out = append(out, line)
	}
	if group == "[Desktop Entry]" && !set {
		out = append(out, "Icon="+name)
		changed = true
	}
	return strings.Join(out, "\n") + "\n", changed
}

// iconInstalled reports whether any hicolor size of name exists under the
// XDG data dirs, so the entry never points at an icon that would not resolve.
func iconInstalled(dirs []string, name string) bool {
	for _, dir := range dirs {
		for _, pattern := range []string{"*/apps/" + name + ".png", "scalable/apps/" + name + ".svg"} {
			if m, _ := filepath.Glob(filepath.Join(dir, "icons", "hicolor", pattern)); len(m) > 0 {
				return true
			}
		}
	}
	return false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// writeAtomic replaces path in one rename, so a launcher watching the
// directory never reads a half-written entry.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".quarro-*.desktop.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
