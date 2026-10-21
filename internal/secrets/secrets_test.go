package secrets

import "testing"

// These tests exercise the in-memory fallback path: this sandbox has no OS
// keyring backend (no Secret Service / Keychain / Credential Manager), so
// every Set call is expected to fall back automatically.
func TestSetGetFallback(t *testing.T) {
	s := NewMemoryStore()
	persistent, err := s.Set("profile-1", "hunter2")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if persistent {
		t.Skip(
			"OS keyring is available in this environment; fallback path not exercised",
		)
	}
	got, ok := s.Get("profile-1")
	if !ok || got != "hunter2" {
		t.Fatalf("Get() = %q, %v; want hunter2, true", got, ok)
	}
}

func TestDeleteClearsFallback(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Set("profile-2", "secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Delete("profile-2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("profile-2"); ok {
		t.Fatal("Get() found a value after Delete")
	}
}

func TestGetMissing(t *testing.T) {
	s := NewMemoryStore()
	if _, ok := s.Get("does-not-exist"); ok {
		t.Fatal("Get() on unknown profile should report ok=false")
	}
}

// A named secret must not collide with the profile's own bare-ID-keyed
// password, and two different named secrets on the same profile must not
// collide with each other.
func TestSetNamedDoesNotCollideWithPrimary(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Set("profile-3", "db-password"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := s.SetNamed("profile-3", "ssh-password", "ssh-pass"); err != nil {
		t.Fatalf("SetNamed: %v", err)
	}
	if got, ok := s.Get("profile-3"); !ok || got != "db-password" {
		t.Fatalf("Get() = %q, %v; want db-password, true", got, ok)
	}
	if got, ok := s.GetNamed("profile-3", "ssh-password"); !ok || got != "ssh-pass" {
		t.Fatalf("GetNamed() = %q, %v; want ssh-pass, true", got, ok)
	}
}

func TestSetNamedTwoKindsIndependent(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.SetNamed("profile-4", "ssh-password", "pw"); err != nil {
		t.Fatalf("SetNamed: %v", err)
	}
	if _, err := s.SetNamed("profile-4", "ssh-passphrase", "phrase"); err != nil {
		t.Fatalf("SetNamed: %v", err)
	}
	if got, ok := s.GetNamed("profile-4", "ssh-password"); !ok || got != "pw" {
		t.Fatalf("GetNamed(ssh-password) = %q, %v; want pw, true", got, ok)
	}
	if got, ok := s.GetNamed("profile-4", "ssh-passphrase"); !ok || got != "phrase" {
		t.Fatalf("GetNamed(ssh-passphrase) = %q, %v; want phrase, true", got, ok)
	}
}

func TestDeleteNamedClearsOnlyThatKind(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.SetNamed("profile-5", "ssh-password", "pw"); err != nil {
		t.Fatalf("SetNamed: %v", err)
	}
	if _, err := s.SetNamed("profile-5", "ssh-passphrase", "phrase"); err != nil {
		t.Fatalf("SetNamed: %v", err)
	}
	if err := s.DeleteNamed("profile-5", "ssh-password"); err != nil {
		t.Fatalf("DeleteNamed: %v", err)
	}
	if _, ok := s.GetNamed("profile-5", "ssh-password"); ok {
		t.Fatal("GetNamed(ssh-password) found a value after DeleteNamed")
	}
	if got, ok := s.GetNamed("profile-5", "ssh-passphrase"); !ok || got != "phrase" {
		t.Fatalf("GetNamed(ssh-passphrase) = %q, %v; want phrase, true (unaffected by the other kind's delete)", got, ok)
	}
}
