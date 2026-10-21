// Package app hosts the Wails-bound application services.
package app

import (
	"fmt"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/driver"
	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/secrets"
	"github.com/yazeed1s/quarro/internal/sessions"
)

// TestResult reports the outcome of a Test Connection attempt.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	// SSHHostFingerprint is set whenever the attempt tunneled through SSH,
	// regardless of whether the profile already had a trusted fingerprint:
	// on a first connect (none trusted yet) it is the key to confirm; on a
	// later connect it already matches what was trusted, since a mismatch
	// fails the attempt outright before reaching here. The frontend only
	// needs to prompt when the profile's own SSHHostFingerprint is empty.
	SSHHostFingerprint string `json:"sshHostFingerprint,omitempty"`
}

// ConnectResult reports the outcome of connecting a session.
type ConnectResult struct {
	OK                 bool   `json:"ok"`
	Message            string `json:"message"`
	SSHHostFingerprint string `json:"sshHostFingerprint,omitempty"`
}

// ConnectionsService is bound to the frontend via Wails: it owns saved
// connection profiles, their secrets, and the live per-session database
// handles.
type ConnectionsService struct {
	profiles *connections.Store
	secrets  *secrets.Store
	sessions *sessions.Registry
}

func NewConnectionsService(
	profiles *connections.Store,
	secretStore *secrets.Store,
	registry *sessions.Registry,
) *ConnectionsService {
	return &ConnectionsService{
		profiles: profiles,
		secrets:  secretStore,
		sessions: registry,
	}
}

// SaveProfileResult wraps the persisted profile with a warning for the
// frontend when a supplied password could not be stored in the OS keyring.
type SaveProfileResult struct {
	Profile               connections.Profile `json:"profile"`
	PasswordNotPersistent bool                `json:"passwordNotPersistent"`
}

// Named secret kinds stored via secrets.Store.SetNamed/GetNamed/DeleteNamed,
// alongside a profile's primary (bare-ID-keyed) password.
const (
	secretSSHPassword   = "ssh-password"
	secretSSHPassphrase = "ssh-passphrase"
)

func (s *ConnectionsService) ListProfiles() ([]connections.Profile, error) {
	return s.profiles.List()
}

// ListDrivers reports every registered engine's display metadata and
// connection-form schema, so the frontend can render the "new connection"
// engine picker and form from backend truth instead of a hardcoded list.
func (s *ConnectionsService) ListDrivers() []driver.DriverMetadata {
	return driver.List()
}

// SaveProfile persists profile metadata and, if a password was supplied,
// routes it to the OS credential store. The plaintext password is never
// written to the profiles file and never returned.
func (s *ConnectionsService) SaveProfile(
	input connections.Input,
) (SaveProfileResult, error) {
	p := input.Profile
	if err := provider.Validate(p); err != nil {
		return SaveProfileResult{}, err
	}
	if err := validateConfigSchema(p); err != nil {
		return SaveProfileResult{}, err
	}
	notPersistent := false
	if input.Password != "" {
		persistent, err := s.secrets.Set(p.ID, input.Password)
		if err != nil {
			return SaveProfileResult{}, fmt.Errorf(
				"store password: %w",
				err,
			)
		}
		p.HasPassword = true
		notPersistent = !persistent
	}
	if input.SSHPassword != "" {
		persistent, err := s.secrets.SetNamed(p.ID, secretSSHPassword, input.SSHPassword)
		if err != nil {
			return SaveProfileResult{}, fmt.Errorf(
				"store SSH password: %w",
				err,
			)
		}
		p.SSHHasPassword = true
		notPersistent = notPersistent || !persistent
	}
	if input.SSHPassphrase != "" {
		persistent, err := s.secrets.SetNamed(p.ID, secretSSHPassphrase, input.SSHPassphrase)
		if err != nil {
			return SaveProfileResult{}, fmt.Errorf(
				"store SSH key passphrase: %w",
				err,
			)
		}
		p.SSHHasPassphrase = true
		notPersistent = notPersistent || !persistent
	}
	ss, err := s.profiles.Save(p)
	if err != nil {
		return SaveProfileResult{}, err
	}
	return SaveProfileResult{
		Profile:               ss,
		PasswordNotPersistent: notPersistent,
	}, nil
}

func (s *ConnectionsService) DeleteProfile(id string) error {
	ps, err := s.profiles.List()
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.ID != id {
			continue
		}
		if p.HasPassword {
			if err := s.secrets.Delete(id); err != nil {
				return err
			}
		}
		if p.SSHHasPassword {
			if err := s.secrets.DeleteNamed(id, secretSSHPassword); err != nil {
				return err
			}
		}
		if p.SSHHasPassphrase {
			if err := s.secrets.DeleteNamed(id, secretSSHPassphrase); err != nil {
				return err
			}
		}
	}
	return s.profiles.Delete(id)
}

// TestConnection attempts a connection without persisting the profile or
// registering a session. Only SQLite actually connects in this phase;
// PostgreSQL and MySQL profiles can be saved but not yet tested/connected.
func (s *ConnectionsService) credentials(
	i connections.Input,
) (connections.Input, error) {
	if i.Password == "" && i.HasPassword {
		var ok bool
		i.Password, ok = s.secrets.Get(i.ID)
		if !ok {
			return i, fmt.Errorf(
				"saved password is unavailable; enter it again",
			)
		}
	}
	if i.SSHEnabled {
		if i.SSHPassword == "" && i.SSHHasPassword {
			var ok bool
			i.SSHPassword, ok = s.secrets.GetNamed(i.ID, secretSSHPassword)
			if !ok {
				return i, fmt.Errorf(
					"saved SSH password is unavailable; enter it again",
				)
			}
		}
		if i.SSHPassphrase == "" && i.SSHHasPassphrase {
			var ok bool
			i.SSHPassphrase, ok = s.secrets.GetNamed(i.ID, secretSSHPassphrase)
			if !ok {
				return i, fmt.Errorf(
					"saved SSH key passphrase is unavailable; enter it again",
				)
			}
		}
	}
	return i, nil
}

func (s *ConnectionsService) TestConnection(
	input connections.Input,
) TestResult {
	input, e := s.credentials(input)
	if e != nil {
		return TestResult{Message: e.Error()}
	}
	engine, e := provider.Get(input.Engine)
	if e != nil {
		return TestResult{Message: e.Error()}
	}
	p, e := requireConnector(engine)
	if e != nil {
		return TestResult{Message: e.Error()}
	}
	db, e := p.Open(input)
	if e != nil {
		return TestResult{Message: e.Error()}
	}
	fp, _ := dialect.TunnelFingerprint(db)
	_ = p.Close(db)
	return TestResult{OK: true, Message: "Connected successfully.", SSHHostFingerprint: fp}
}

func (s *ConnectionsService) ImportURL(
	raw string,
) (connections.Input, error) {
	return provider.ImportURL(raw)
}

// Connect opens (or re-opens) the database handle for sessionID against
// profileID and registers it in the session registry.
func (s *ConnectionsService) Connect(
	sessionID, profileID string,
) (ConnectResult, error) {
	return s.ConnectTarget(sessionID, profileID, "")
}

func (s *ConnectionsService) ConnectTarget(
	sessionID, profileID, database string,
) (ConnectResult, error) {
	generation := s.sessions.BeginConnect(sessionID)
	profiles, err := s.profiles.List()
	if err != nil {
		return ConnectResult{}, err
	}
	var profile *connections.Profile
	for i := range profiles {
		if profiles[i].ID == profileID {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return ConnectResult{
			OK:      false,
			Message: "This connection profile no longer exists.",
		}, nil
	}
	if database != "" && profile.Engine != connections.EngineSQLite {
		profile.Database = database
	}
	return s.open(sessionID, *profile, generation)
}

func (s *ConnectionsService) open(
	id string,
	profile connections.Profile,
	generation uint64,
) (ConnectResult, error) {
	input, e := s.credentials(connections.Input{Profile: profile})
	if e != nil {
		return ConnectResult{Message: e.Error()}, nil
	}
	engine, e := provider.Get(profile.Engine)
	if e != nil {
		return ConnectResult{Message: e.Error()}, nil
	}
	p, e := requireConnector(engine)
	if e != nil {
		return ConnectResult{Message: e.Error()}, nil
	}
	db, e := p.Open(input)
	if e != nil {
		return ConnectResult{Message: e.Error()}, nil
	}
	fp, _ := dialect.TunnelFingerprint(db)
	if !s.sessions.InstallProvider(
		id,
		profile,
		generation,
		db,
		engine,
	) {
		return ConnectResult{
			Message: "Connection attempt superseded.",
		}, nil
	}
	return ConnectResult{OK: true, SSHHostFingerprint: fp}, nil
}

// OpenDatabase always creates a separate session; the source keeps its target.
func (s *ConnectionsService) OpenDatabase(
	sourceID, newID, database string,
) (ConnectResult, error) {
	if sourceID == newID || s.sessions.Connected(newID) {
		return ConnectResult{}, fmt.Errorf(
			"database changes require a new session ID",
		)
	}
	profile, e := s.sessions.Profile(sourceID)
	if e != nil {
		return ConnectResult{}, e
	}
	if profile.Engine == connections.EngineSQLite {
		return ConnectResult{}, fmt.Errorf(
			"open another SQLite profile instead",
		)
	}
	profile.Database = database
	return s.open(newID, profile, s.sessions.BeginConnect(newID))
}

func (s *ConnectionsService) Disconnect(sessionID string) error {
	return s.sessions.Disconnect(sessionID)
}
