package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

const entry = `[Desktop Entry]
Type=Application
Name=Quarro
Exec=quarro
Icon=quarro

[Desktop Action new]
Name=New window
Icon=other
`

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// installed lays out a system data dir as the packages do: the entry plus
// all three named icons.
func installed(t *testing.T) (home, system string) {
	home, system = t.TempDir(), t.TempDir()
	write(t, filepath.Join(system, "applications", launcherEntry), entry)
	for _, name := range []string{"quarro", "quarro-dark", "quarro-light"} {
		write(t, filepath.Join(system, "icons/hicolor/48x48/apps", name+".png"), "png")
	}
	return home, system
}

func TestWithIcon(t *testing.T) {
	got, changed := withIcon(entry, "quarro-light", false)
	if !changed || !strings.Contains(got, "Exec=quarro\nIcon=quarro-light\n") || !strings.Contains(got, "Icon=other") {
		t.Fatalf("changed %v:\n%s", changed, got)
	}
	if _, changed := withIcon(entry, "quarro-dark", true); changed {
		t.Fatal("Icon=quarro rewritten for the dark (shipped) icon")
	}
	if _, changed := withIcon(got, "quarro-light", false); changed {
		t.Fatal("unchanged icon reported as changed")
	}
	got, changed = withIcon("[Desktop Entry]\nName=Quarro\n[Desktop Action x]\nName=X\n", "quarro-light", false)
	if !changed || !strings.Contains(got, "Name=Quarro\nIcon=quarro-light\n[Desktop Action x]") {
		t.Fatalf("missing Icon= not added to the entry group:\n%s", got)
	}
}

func TestSetLauncherIconCopiesSystemEntry(t *testing.T) {
	home, system := installed(t)
	user := filepath.Join(home, "applications", launcherEntry)
	// Dark on a fresh install leaves the system entry alone.
	if err := setLauncherIcon(home, []string{system}, windowprefs.AppIconDark); err != nil {
		t.Fatal(err)
	}
	if fileExists(user) {
		t.Fatal("dark choice created a user entry")
	}
	if err := setLauncherIcon(home, []string{system}, windowprefs.AppIconLight); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(user)
	if !strings.Contains(string(data), "Icon=quarro-light\n") || !strings.Contains(string(data), "Exec=quarro\n") {
		t.Fatalf("user entry:\n%s", data)
	}
	// Back to dark edits the user entry, which now wins over the system one.
	if err := setLauncherIcon(home, []string{system}, windowprefs.AppIconDark); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(user); !strings.Contains(string(data), "Icon=quarro-dark\n") {
		t.Fatalf("user entry:\n%s", data)
	}
	if left, _ := filepath.Glob(filepath.Join(home, "applications", ".quarro-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

func TestSetLauncherIconFailures(t *testing.T) {
	home, system := installed(t)
	os.Remove(filepath.Join(system, "icons/hicolor/48x48/apps/quarro-light.png"))
	if err := setLauncherIcon(home, []string{system}, windowprefs.AppIconLight); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing icon: %v", err)
	}
	home, system = installed(t)
	os.Remove(filepath.Join(system, "applications", launcherEntry))
	if err := setLauncherIcon(home, []string{system}, windowprefs.AppIconLight); err == nil || !strings.Contains(err.Error(), launcherEntry) {
		t.Fatalf("missing entry: %v", err)
	}
}
