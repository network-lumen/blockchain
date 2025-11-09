package keys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	storageDirName  = "pqc_keys"
	keysFileName    = "keys.json"
	linksFileName   = "links.json"
	defaultFileMode = 0o600
)

// KeyRecord describes a locally stored PQC key pair.
type KeyRecord struct {
	Name       string    `json:"name"`
	Scheme     string    `json:"scheme"`
	PublicKey  []byte    `json:"public_key"`
	PrivateKey []byte    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store manages local PQC keys and address bindings.
type Store struct {
	dir       string
	keysPath  string
	linksPath string

	mu    sync.RWMutex
	keys  map[string]KeyRecord
	links map[string]string
}

// LoadStore initialises a PQC key store under the provided home directory.
func LoadStore(homeDir string) (*Store, error) {
	if homeDir == "" {
		return nil, errors.New("home directory is required")
	}

	dir := filepath.Join(homeDir, storageDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create pqc key directory: %w", err)
	}

	store := &Store{
		dir:       dir,
		keysPath:  filepath.Join(dir, keysFileName),
		linksPath: filepath.Join(dir, linksFileName),
		keys:      make(map[string]KeyRecord),
		links:     make(map[string]string),
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

// load reads the keys and links data from disk.
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadKeysLocked(); err != nil {
		return err
	}
	if err := s.loadLinksLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) loadKeysLocked() error {
	data, err := os.ReadFile(s.keysPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read pqc keys: %w", err)
	}

	var raw map[string]KeyRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal pqc keys: %w", err)
	}
	s.keys = raw
	return nil
}

func (s *Store) loadLinksLocked() error {
	data, err := os.ReadFile(s.linksPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read pqc links: %w", err)
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal pqc links: %w", err)
	}
	s.links = raw
	return nil
}

// SaveKey stores or replaces a PQC key.
func (s *Store) SaveKey(record KeyRecord) error {
	if record.Name == "" {
		return errors.New("key name cannot be empty")
	}
	if len(record.PublicKey) == 0 {
		return errors.New("public key cannot be empty")
	}
	if len(record.PrivateKey) == 0 {
		return errors.New("private key cannot be empty")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.keys[record.Name] = record
	return s.persistLocked(s.keysPath, s.keys)
}

// GetKey retrieves a key by name.
func (s *Store) GetKey(name string) (KeyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.keys[name]
	return record, ok
}

// ListKeys returns all stored key records.
func (s *Store) ListKeys() []KeyRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]KeyRecord, 0, len(s.keys))
	for _, rec := range s.keys {
		out = append(out, rec)
	}
	return out
}

// LinkAddress associates a cosmos address with a local PQC key name.
func (s *Store) LinkAddress(address, keyName string) error {
	if address == "" {
		return errors.New("address cannot be empty")
	}
	if keyName == "" {
		return errors.New("key name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.links[address] = keyName
	return s.persistLocked(s.linksPath, s.links)
}

// GetLink returns the PQC key name linked to the address.
func (s *Store) GetLink(address string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name, ok := s.links[address]
	return name, ok
}

// ListLinks returns all local bindings.
func (s *Store) ListLinks() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]string, len(s.links))
	for addr, name := range s.links {
		out[addr] = name
	}
	return out
}

func (s *Store) persistLocked(path string, v any) error {
	tmpFile, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("encode pqc store: %w", err)
	}
	if err := tmpFile.Chmod(defaultFileMode); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod tmp store: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp store: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("persist pqc store: %w", err)
	}
	return nil
}
