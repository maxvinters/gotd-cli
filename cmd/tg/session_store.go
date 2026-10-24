package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-faster/errors"

	"github.com/gotd/td/session"

	"github.com/gotd/cli/internal/privatefile"
)

// sessionService is the Keychain service name under which sessions are stored.
// It mirrors the on-disk session filename prefix (gotd.session.*).
const sessionService = "gotd.session"

// sessionStore persists a gotd string session. It is the single seam through
// which the session is read (by the gotd client), checked for existence (by
// `accounts`), and removed (by `logout`). Backends: a plain file, or the macOS
// Keychain (default on darwin).
type sessionStore interface {
	session.Storage // LoadSession / StoreSession, consumed by telegram.Options.
	Exists(ctx context.Context) (bool, error)
	Delete(ctx context.Context) error
}

// fileSessionStore is the cross-platform backend: gotd's JSON FileStorage plus
// existence/deletion over the same path.
type fileSessionStore struct {
	*session.FileStorage
	mu sync.Mutex
}

func (s *fileSessionStore) Exists(context.Context) (bool, error) {
	info, err := os.Lstat(s.Path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return false, errors.Errorf("session path %s is not a regular file", s.Path)
		}
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

func (s *fileSessionStore) LoadSession(context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := privatefile.Read(s.Path)
	if os.IsNotExist(err) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "read session")
	}
	return data, nil
}

func (s *fileSessionStore) StoreSession(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := privatefile.Write(s.Path, data); err != nil {
		return errors.Wrap(err, "write session")
	}
	return nil
}

func (s *fileSessionStore) Delete(context.Context) error {
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// keychainAccount is the Keychain account name (and matches the file seed) for a
// given account label + auth kind, so user/bot and distinct app credentials
// never collide.
func keychainAccount(label string, acc Account, kind string) string {
	return label + "." + kind + "." + acc.seed(label, kind)
}

// keychainService namespaces entries by the full config path so separate config
// files cannot share sessions.
func keychainService(configPath string) string {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		configPath = filepath.Clean(configPath)
	}
	sum := sha256.Sum256([]byte(configPath))
	return sessionService + "." + hex.EncodeToString(sum[:])
}

// newSessionStore selects the backend. On macOS with Keychain enabled (the
// default) it returns the Keychain backend; otherwise it falls back to a file in
// the config directory.
func newSessionStore(configPath, label string, acc Account, kind string, useKeychain bool) sessionStore {
	path := acc.sessionPath(filepath.Dir(configPath), label, kind)
	if useKeychain {
		// Only the scoped legacy file migrates; the old global Keychain item is
		// intentionally left untouched because its config identity is ambiguous.
		if ks, ok := keychainSessionStore(keychainService(configPath), keychainAccount(label, acc, kind), path); ok {
			return ks
		}
	}
	return &fileSessionStore{FileStorage: &session.FileStorage{Path: path}}
}

// useKeychain reports whether the Keychain backend is enabled. It defaults to
// on; set `keychain: false` in the config to opt out (e.g. headless macOS).
func (a *app) useKeychain() bool {
	return a.cfg.Keychain == nil || *a.cfg.Keychain
}

// sessionStore builds the session store for an account label + auth kind.
func (a *app) sessionStore(label string, acc Account, kind string) sessionStore {
	return newSessionStore(a.configPath, label, acc, kind, a.useKeychain())
}
