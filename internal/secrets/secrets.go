// Package secrets stores connection profile passwords in the OS credential
// store, falling back to an in-memory, non-persistent map when no OS keyring
// is available (e.g. headless Linux with no Secret Service running). Secrets
// are never written to plain files.
package secrets

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

const service = "quarro"

// Store persists a secret per profile ID.
type Store struct {
	mu         sync.Mutex
	fallback   map[string]string
	namespace  string
	memoryOnly bool
}

func NewStore() *Store {
	return NewStoreForNamespace(service)
}

// Set stores password for profileID. Persistent reports whether the OS
// keyring accepted it; when false, the password only lives for the life of
// this process and the caller should tell the user it won't survive restart.
func (s *Store) Set(
	profileID, password string,
) (persistent bool, err error) {
	if !s.memoryOnly {
		if err := keyring.Set(s.namespace, profileID, password); err == nil {
			s.mu.Lock()
			delete(s.fallback, profileID)
			s.mu.Unlock()
			return true, nil
		}
	}
	s.mu.Lock()
	s.fallback[profileID] = password
	s.mu.Unlock()
	return false, nil
}

// Get returns the stored password for profileID, if any.
func (s *Store) Get(profileID string) (password string, ok bool) {
	s.mu.Lock()
	cached, exists := s.fallback[profileID]
	s.mu.Unlock()
	if exists {
		return cached, true
	}
	if s.memoryOnly {
		return "", false
	}
	if v, err := keyring.Get(s.namespace, profileID); err == nil {
		return v, true
	}
	s.mu.Lock()
	v, ok := s.fallback[profileID]
	s.mu.Unlock()
	return v, ok
}

// Delete removes any stored password for profileID, from both the OS
// keyring and the in-memory fallback.
func (s *Store) Delete(profileID string) error {
	s.mu.Lock()
	delete(s.fallback, profileID)
	s.mu.Unlock()
	if s.memoryOnly {
		return nil
	}
	if err := keyring.Delete(s.namespace, profileID); err != nil &&
		!errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func NewStoreForNamespace(namespace string) *Store {
	return &Store{fallback: map[string]string{}, namespace: namespace}
}

// SetNamed, GetNamed, and DeleteNamed store a secondary secret for a
// profile (an SSH password or private-key passphrase) under its own
// keyring entry, distinct from the profile's primary password. kind is a
// short fixed string ("ssh-password", "ssh-passphrase"); the composed key
// can never collide with a bare profileID, so one profile can hold several
// independent named secrets without a second Store.
func (s *Store) SetNamed(profileID, kind, secret string) (persistent bool, err error) {
	return s.Set(namedKey(profileID, kind), secret)
}

func (s *Store) GetNamed(profileID, kind string) (secret string, ok bool) {
	return s.Get(namedKey(profileID, kind))
}

func (s *Store) DeleteNamed(profileID, kind string) error {
	return s.Delete(namedKey(profileID, kind))
}

func namedKey(profileID, kind string) string {
	return profileID + ":" + kind
}

// NewMemoryStore provides deterministic tests without accessing the user's keyring.
func NewMemoryStore() *Store { s := NewStore(); s.memoryOnly = true; return s }
