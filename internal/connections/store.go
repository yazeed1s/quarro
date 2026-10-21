// Package connections owns saved connection profiles: their non-secret
// metadata persisted to a JSON file, and their passwords routed through
// internal/secrets so no plaintext password is ever written to that file.
package connections

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

type Engine string

const (
	EngineSQLite   Engine = "sqlite"
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
	EngineMongoDB  Engine = "mongodb"
	EngineRedis    Engine = "redis"
)

// Profile is the persisted, non-secret record of a saved connection.
type Profile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Engine        Engine `json:"engine"`
	Database      string `json:"database"`
	Schema        string `json:"schema"`
	Environment   string `json:"environment"`
	Folder        string `json:"folder"`
	Favorite      bool   `json:"favorite"`
	Host          string `json:"host"`
	Port          string `json:"port"`
	Username      string `json:"username"`
	Path          string `json:"path"`
	ReadOnly      bool   `json:"readOnly"`
	TLSMode       string `json:"tlsMode"`
	TLSCA         string `json:"tlsCA"`
	TLSCert       string `json:"tlsCert"`
	TLSKey        string `json:"tlsKey"`
	TLSServerName string `json:"tlsServerName"`
	Socket        string `json:"socket"`
	HasPassword   bool   `json:"hasPassword"`

	// SSH tunnel fields. Only meaningful for PostgreSQL/MySQL profiles;
	// see internal/database/dialect.ValidateSSHProfile for the engine and
	// mutual-exclusion (with Socket) checks. SSHPassword/SSHPassphrase are
	// secrets and live in Input, not here, the same way Password does.
	SSHEnabled        bool   `json:"sshEnabled,omitempty"`
	SSHHost           string `json:"sshHost,omitempty"`
	SSHPort           string `json:"sshPort,omitempty"`
	SSHUsername       string `json:"sshUsername,omitempty"`
	SSHAuthMethod     string `json:"sshAuthMethod,omitempty"` // "password" | "key"
	SSHPrivateKeyPath string `json:"sshPrivateKeyPath,omitempty"`
	SSHHasPassword    bool   `json:"sshHasPassword,omitempty"`
	SSHHasPassphrase  bool   `json:"sshHasPassphrase,omitempty"`
	// SSHHostFingerprint is the SHA256 fingerprint (ssh.FingerprintSHA256
	// format, "SHA256:...") accepted at the most recent successful
	// connect. Non-secret: a public key fingerprint discloses nothing
	// about the private key. Empty means no host has been trusted yet.
	SSHHostFingerprint string `json:"sshHostFingerprint,omitempty"`

	// Config holds a driver's own connection fields that this struct has
	// no named field for: a plugin driver whose ConfigSchema (see
	// internal/database/driver.DriverMetadata) declares a key beyond
	// host/port/database/username/path/socket/TLS/readOnly stores it here
	// instead of requiring a change to this struct.
	Config map[string]string `json:"config,omitempty"`
}

// Input is what the frontend submits for Save/Test/Connect. Password is
// optional and, on Save, is written to the secret store rather than here.
type Input struct {
	Profile
	Password      string `json:"password,omitempty"`
	SSHPassword   string `json:"sshPassword,omitempty"`
	SSHPassphrase string `json:"sshPassphrase,omitempty"`
}

// Store persists profiles as a single JSON file under the user's config
// directory.
type Store struct {
	mu   sync.Mutex
	path string
}

func NewStore() (*Store, error) {
	return NewStoreForStage("quarro", "")
}

// NewStoreForStage returns a Store whose profiles live in dir (under the
// user's config directory) as "profiles<suffix>.json", so different build
// stages can share one directory without clobbering each other's profiles.
func NewStoreForStage(dir, suffix string) (*Store, error) {
	path, err := xdg.ConfigFile(
		filepath.Join(dir, "profiles"+suffix+".json"),
	)
	if err != nil {
		return nil, fmt.Errorf("resolve profiles path: %w", err)
	}
	return NewStoreAt(path), nil
}

// NewStoreAt returns a Store backed by an explicit file path, primarily for
// tests that need an isolated location instead of the real config directory.
func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Save inserts or replaces the profile with the same ID and returns the
// stored (non-secret) record.
func (s *Store) Save(p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	replaced := false
	for i, existing := range profiles {
		if existing.ID == p.ID {
			profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, p)
	}
	if err := s.write(profiles); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.load()
	if err != nil {
		return err
	}
	kept := profiles[:0]
	for _, p := range profiles {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	return s.write(kept)
}

func (s *Store) load() ([]Profile, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []Profile{}, nil
	}
	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *Store) write(profiles []Profile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".profiles-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
