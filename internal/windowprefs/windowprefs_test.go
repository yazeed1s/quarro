package windowprefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		contents string
		write    bool
		want     Prefs
		wantErr  bool
	}{
		"missing file": {want: Default()},
		"corrupt json": {write: true, contents: "{", want: Default(), wantErr: true},
		"unknown title bar keeps valid background": {
			write:    true,
			contents: `{"titleBar":"fancy","background":"#f4f5f5"}`,
			want:     Prefs{TitleBar: TitleBarSystem, Background: "#f4f5f5", AppIcon: AppIconDark},
		},
		"bad background keeps valid title bar": {
			write:    true,
			contents: `{"titleBar":"custom","background":"red"}`,
			want:     Prefs{TitleBar: TitleBarCustom, Background: DefaultBackground, AppIcon: AppIconDark},
		},
		"unknown app icon keeps the rest": {
			write:    true,
			contents: `{"titleBar":"custom","background":"#f4f5f5","appIcon":"neon"}`,
			want:     Prefs{TitleBar: TitleBarCustom, Background: "#f4f5f5", AppIcon: AppIconDark},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if tc.write {
				if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := NewStoreAt(path).Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSaveRoundTrip(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "nested", "window.json"))
	want := Prefs{TitleBar: TitleBarCustom, Background: "#f4f5f5", AppIcon: AppIconLight}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got != want {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "window.json"))
	for _, p := range []Prefs{
		{TitleBar: "fancy", Background: DefaultBackground, AppIcon: AppIconDark},
		{TitleBar: TitleBarSystem, Background: "#fff", AppIcon: AppIconDark},
		{TitleBar: TitleBarSystem, Background: DefaultBackground, AppIcon: "neon"},
	} {
		if err := store.Save(p); err == nil {
			t.Fatalf("Save(%+v) succeeded", p)
		}
	}
}

func TestFrameless(t *testing.T) {
	for _, tc := range []struct {
		goos string
		bar  TitleBar
		want bool
	}{
		{"linux", TitleBarSystem, false},
		{"linux", TitleBarCustom, true},
		{"darwin", TitleBarSystem, false},
		{"darwin", TitleBarCustom, false},
		{"windows", TitleBarSystem, true},
	} {
		if got := Frameless(tc.goos, tc.bar); got != tc.want {
			t.Errorf("Frameless(%s, %s) = %v", tc.goos, tc.bar, got)
		}
	}
}

func TestParseHex(t *testing.T) {
	r, g, b, ok := ParseHex("#0d0e0f")
	if !ok || r != 0x0d || g != 0x0e || b != 0x0f {
		t.Fatalf("got %x %x %x %v", r, g, b, ok)
	}
	for _, bad := range []string{"", "0d0e0f", "#0d0e0", "#0d0e0fz", "#zzzzzz"} {
		if _, _, _, ok := ParseHex(bad); ok {
			t.Errorf("ParseHex(%q) ok", bad)
		}
	}
}

func TestAppIconSwappable(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "windows": true, "linux": true, "freebsd": false} {
		if got := AppIconSwappable(goos); got != want {
			t.Errorf("AppIconSwappable(%s) = %v", goos, got)
		}
	}
}
