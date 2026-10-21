package app

import (
	"path/filepath"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/secrets"
	"github.com/yazeed1s/quarro/internal/sessions"
)

func newTestService(t *testing.T) *ConnectionsService {
	t.Helper()
	store := connections.NewStoreAt(
		filepath.Join(t.TempDir(), "profiles.json"),
	)
	return NewConnectionsService(
		store,
		secrets.NewMemoryStore(),
		sessions.NewRegistry(),
	)
}

func TestSaveAndListProfile(t *testing.T) {
	svc := newTestService(t)
	result, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{
			ID:     "p1",
			Name:   "Local",
			Engine: connections.EngineSQLite,
			Path:   "/tmp/unused-saved.db",
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if result.Profile.ID != "p1" {
		t.Fatalf("SaveProfile() = %+v", result)
	}
	list, err := svc.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListProfiles() = %+v, want 1 profile", list)
	}
}

func TestSaveProfileWithPasswordSetsHasPassword(t *testing.T) {
	svc := newTestService(t)
	result, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{
			ID:       "p1",
			Name:     "Dev",
			Engine:   connections.EnginePostgres,
			Database: "test",
			Username: "test",
			Host:     "localhost",
		},
		Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if !result.Profile.HasPassword {
		t.Fatal(
			"expected HasPassword to be true after saving with a password",
		)
	}
}

func TestConnectAndDisconnectSQLite(t *testing.T) {
	svc := newTestService(t)
	path := filepath.Join(t.TempDir(), "session.db")
	if _, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{ID: "p1", Name: "Local", Engine: connections.EngineSQLite, Path: path},
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	result, err := svc.Connect("session-1", "p1")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !result.OK {
		t.Fatalf("Connect() = %+v, want OK", result)
	}
	if !svc.sessions.Connected("session-1") {
		t.Fatal("expected session-1 to be registered as connected")
	}

	if err := svc.Disconnect("session-1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if svc.sessions.Connected("session-1") {
		t.Fatal("expected session-1 to be disconnected")
	}
}

func TestConnectUnknownProfile(t *testing.T) {
	svc := newTestService(t)
	result, err := svc.Connect("session-1", "does-not-exist")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if result.OK {
		t.Fatal(
			"Connect() to an unknown profile should not report OK",
		)
	}
}

func TestTestConnectionSQLite(t *testing.T) {
	svc := newTestService(t)
	path := filepath.Join(t.TempDir(), "test.db")
	result := svc.TestConnection(connections.Input{
		Profile: connections.Profile{
			Engine: connections.EngineSQLite,
			Path:   path,
		},
	})
	if !result.OK {
		t.Fatalf("TestConnection() = %+v, want OK", result)
	}
}

func TestTestConnectionReadOnlyMissingFileFails(t *testing.T) {
	svc := newTestService(t)
	path := filepath.Join(t.TempDir(), "missing.db")
	result := svc.TestConnection(connections.Input{
		Profile: connections.Profile{
			Engine:   connections.EngineSQLite,
			Path:     path,
			ReadOnly: true,
		},
	})
	if result.OK {
		t.Fatal(
			"TestConnection() on a missing read-only file should fail",
		)
	}
}

func TestTestConnectionUnsupportedEngine(t *testing.T) {
	svc := newTestService(t)
	result := svc.TestConnection(connections.Input{
		Profile: connections.Profile{
			Engine:   connections.EnginePostgres,
			Database: "test",
			Username: "test",
			Host:     "localhost",
		},
	})
	if result.OK {
		t.Fatal(
			"TestConnection() for postgres should not claim success in this phase",
		)
	}
}

func TestSaveProfileWithSSHSecretsSetsHasFlags(t *testing.T) {
	svc := newTestService(t)
	result, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{
			ID: "p1", Name: "Dev", Engine: connections.EnginePostgres,
			Database: "test", Username: "test", Host: "localhost",
			SSHEnabled: true, SSHHost: "jump.example.com",
			SSHUsername: "deploy", SSHAuthMethod: "key",
			SSHPrivateKeyPath: "/home/u/.ssh/id_ed25519",
		},
		SSHPassphrase: "correct horse",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if !result.Profile.SSHHasPassphrase {
		t.Fatal("expected SSHHasPassphrase to be true after saving with a passphrase")
	}
	if result.Profile.SSHHasPassword {
		t.Fatal("expected SSHHasPassword to stay false when no SSH password was supplied")
	}
}

func TestCredentialsResolvesSavedSSHSecrets(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{
			ID: "p1", Name: "Dev", Engine: connections.EnginePostgres,
			Database: "test", Username: "test", Host: "localhost",
			SSHEnabled: true, SSHHost: "jump.example.com",
			SSHUsername: "deploy", SSHAuthMethod: "password",
		},
		SSHPassword: "tunnel-secret",
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	resolved, err := svc.credentials(connections.Input{
		Profile: connections.Profile{
			ID: "p1", SSHEnabled: true, SSHHasPassword: true,
		},
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if resolved.SSHPassword != "tunnel-secret" {
		t.Fatalf("credentials() SSHPassword = %q, want tunnel-secret", resolved.SSHPassword)
	}
}

func TestCredentialsMissingSavedSSHSecretFails(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.credentials(connections.Input{
		Profile: connections.Profile{
			ID: "does-not-exist", SSHEnabled: true, SSHHasPassword: true,
		},
	})
	if err == nil {
		t.Fatal("credentials() should fail when a flagged SSH secret was never stored")
	}
}

func TestDeleteProfileRemovesSSHSecrets(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.SaveProfile(connections.Input{
		Profile: connections.Profile{
			ID: "p1", Name: "Dev", Engine: connections.EnginePostgres,
			Database: "test", Username: "test", Host: "localhost",
			SSHEnabled: true, SSHHost: "jump.example.com",
			SSHUsername: "deploy", SSHAuthMethod: "key",
			SSHPrivateKeyPath: "/home/u/.ssh/id_ed25519",
		},
		SSHPassphrase: "correct horse",
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := svc.DeleteProfile("p1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, ok := svc.secrets.GetNamed("p1", secretSSHPassphrase); ok {
		t.Fatal("expected the stored SSH passphrase to be gone after DeleteProfile")
	}
}

func TestDeleteProfileRemovesSecret(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.SaveProfile(connections.Input{
		Profile:  connections.Profile{ID: "p1", Name: "Dev", Engine: connections.EnginePostgres, Database: "test", Username: "test", Host: "localhost"},
		Password: "hunter2",
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := svc.DeleteProfile("p1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, ok := svc.secrets.Get("p1"); ok {
		t.Fatal(
			"expected the stored password to be gone after DeleteProfile",
		)
	}
	list, err := svc.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf(
			"ListProfiles() = %+v, want empty after delete",
			list,
		)
	}
}
