// Package secrets resolves BYO provider keys from a local secret store
// (OS keyring or a user-owned 0600 file) before falling back to an
// environment variable (ADR-0003, constitution section 10.5). Keys are
// never logged or committed.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Source names where a resolved secret came from.
const (
	SourceNone    = "none"
	SourceKeyring = "keyring"
	SourceEnv     = "env"
)

// ErrNotFound is returned when a store has no value for the requested key.
var ErrNotFound = errors.New("secret not found")

// Store is a local secret backend. Implementations must not transmit
// values off the machine.
type Store interface {
	Get(ctx context.Context, service, account string) (string, error)
	Set(ctx context.Context, service, account, secret string) error
}

// Lookup describes how to resolve one provider key.
type Lookup struct {
	Service string
	Account string
	EnvVar  string
}

// MemoryStore is an in-process store used by tests and as a last-resort
// default when no filesystem is available.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]string
}

// NewMemoryStore builds an empty memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]string{}}
}

func memKey(service, account string) string { return service + "\x00" + account }

// Get returns a previously stored secret.
func (m *MemoryStore) Get(_ context.Context, service, account string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[memKey(service, account)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set stores a secret.
func (m *MemoryStore) Set(_ context.Context, service, account, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[memKey(service, account)] = secret
	return nil
}

// FileStore keeps one 0600 file per service/account under a root directory.
type FileStore struct {
	root string
}

// DefaultStore returns the user-owned file store under the config directory
// (`$XDG_CONFIG_HOME/loka/secrets` or equivalent). It falls back to an
// in-memory store when the home path cannot be resolved so construction
// never fails offline. This is the v1 stand-in for an OS keyring: 0600
// files the user owns, never committed, never logged.
func DefaultStore() Store {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return NewMemoryStore()
	}
	s, err := OpenFileStore(filepath.Join(dir, "loka", "secrets"))
	if err != nil {
		return NewMemoryStore()
	}
	return s
}

// OpenFileStore prepares a file-backed store. The root directory is created
// with 0700 permissions when missing.
func OpenFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("secrets file store root must not be empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create secrets dir: %w", err)
	}
	return &FileStore{root: root}, nil
}

func (f *FileStore) path(service, account string) (string, error) {
	if service == "" || account == "" {
		return "", errors.New("service and account must not be empty")
	}
	if strings.Contains(service, "..") || strings.Contains(account, "..") ||
		strings.ContainsAny(service, `/\`) || strings.ContainsAny(account, `/\`) {
		return "", errors.New("service and account must be path-safe")
	}
	return filepath.Join(f.root, service, account), nil
}

// Get reads a 0600 secret file.
func (f *FileStore) Get(_ context.Context, service, account string) (string, error) {
	p, err := f.path(service, account)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// Set writes a secret as a 0600 file under a 0700 service directory.
func (f *FileStore) Set(_ context.Context, service, account, secret string) error {
	p, err := f.path(service, account)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(secret), 0o600)
}

// ParseKeyringRef splits "service/account" into parts.
func ParseKeyringRef(ref string) (service, account string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", errors.New("empty keyring ref")
	}
	i := strings.IndexByte(ref, '/')
	if i <= 0 || i == len(ref)-1 {
		return "", "", fmt.Errorf("keyring ref %q must be service/account", ref)
	}
	return ref[:i], ref[i+1:], nil
}

// Resolve returns a secret from the store when service/account are set,
// otherwise from EnvVar. Keyring wins over the environment so a user-owned
// store is not shadowed by a leftover shell variable.
func Resolve(ctx context.Context, store Store, lookup Lookup) (secret, source string, err error) {
	if store != nil && lookup.Service != "" && lookup.Account != "" {
		v, err := store.Get(ctx, lookup.Service, lookup.Account)
		if err == nil && v != "" {
			return v, SourceKeyring, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return "", SourceNone, err
		}
	}
	if lookup.EnvVar != "" {
		if v := os.Getenv(lookup.EnvVar); v != "" {
			return v, SourceEnv, nil
		}
	}
	return "", SourceNone, nil
}
