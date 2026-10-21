package connections

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveListRoundTrip(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "profiles.json"))

	saved, err := store.Save(
		Profile{
			ID:     "p1",
			Name:   "Local",
			Engine: EngineSQLite,
			Path:   "/tmp/x.db",
		},
	)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID != "p1" {
		t.Fatalf("Save() returned ID %q, want p1", saved.ID)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Local" {
		t.Fatalf("List() = %+v, want one profile named Local", list)
	}
}

func TestSaveReplacesExistingID(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "profiles.json"))
	if _, err := store.Save(Profile{ID: "p1", Name: "First"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.Save(Profile{ID: "p1", Name: "Renamed"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Renamed" {
		t.Fatalf(
			"List() = %+v, want a single profile named Renamed",
			list,
		)
	}
}

func TestDeleteRemovesProfile(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "profiles.json"))
	if _, err := store.Save(Profile{ID: "p1", Name: "Local"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete("p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() = %+v, want empty after Delete", list)
	}
}

func TestListOnMissingFile(t *testing.T) {
	store := NewStoreAt(
		filepath.Join(t.TempDir(), "does-not-exist", "profiles.json"),
	)
	if _, err := store.List(); err != nil {
		t.Fatalf(
			"List on missing file should return an empty slice, not error: %v",
			err,
		)
	}
}

// A profile's password must never reach the JSON file on diskm only the
// hasPassword flag. Save never even sees a Password field (that lives on
// Input, handled one layer up in the service), so this asserts the file
// contents never contain a "password" key regardless of profile content.
func TestStoredFileHasNoPasswordField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewStoreAt(path)
	if _, err := store.Save(Profile{ID: "p1", Name: "Local", HasPassword: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(strings.ToLower(string(data)), `"password"`) {
		t.Fatalf("profiles.json contains a password field: %s", data)
	}
}
