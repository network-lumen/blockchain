package keys

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/scrypt"
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
type StoreOptions struct {
	passphrase []byte
}

type StoreOption func(*StoreOptions)

// WithPassphrase instructs the store to encrypt key material on disk using the provided passphrase.
func WithPassphrase(passphrase []byte) StoreOption {
	return func(o *StoreOptions) {
		if len(passphrase) == 0 {
			return
		}
		o.passphrase = append([]byte(nil), passphrase...)
	}
}

type Store struct {
	dir        string
	keysPath   string
	linksPath  string
	passphrase []byte

	mu    sync.RWMutex
	keys  map[string]KeyRecord
	links map[string]string
}

// LoadStore initialises a PQC key store under the provided home directory.
func LoadStore(homeDir string, opts ...StoreOption) (*Store, error) {
	if homeDir == "" {
		return nil, errors.New("home directory is required")
	}

	dir := filepath.Join(homeDir, storageDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create pqc key directory: %w", err)
	}

	optValues := StoreOptions{}
	for _, opt := range opts {
		opt(&optValues)
	}

	store := &Store{
		dir:       dir,
		keysPath:  filepath.Join(dir, keysFileName),
		linksPath: filepath.Join(dir, linksFileName),
		passphrase: func() []byte {
			if len(optValues.passphrase) == 0 {
				return nil
			}
			return append([]byte(nil), optValues.passphrase...)
		}(),
		keys:  make(map[string]KeyRecord),
		links: make(map[string]string),
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
	data, err := s.readFileMaybeEncrypted(s.keysPath)
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
	data, err := s.readFileMaybeEncrypted(s.linksPath)
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
	if err := osRenameEncrypted(tmpPath, path, s.passphrase); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("persist pqc store: %w", err)
	}
	return nil
}

const (
	encryptionMagic = "PQCENC1"
	encryptionSalt  = 16
	encryptionNonce = 12
)

func osRenameEncrypted(tmpPath, finalPath string, passphrase []byte) error {
	if len(passphrase) == 0 {
		return os.Rename(tmpPath, finalPath)
	}

	plaintext, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	ciphertext, err := encryptBytes(passphrase, plaintext)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, ciphertext, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

func (s *Store) readFileMaybeEncrypted(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte(encryptionMagic)) {
		if len(s.passphrase) == 0 {
			return nil, fmt.Errorf("%s is encrypted; provide a PQC keystore passphrase", path)
		}
		plain, err := decryptBytes(s.passphrase, data)
		if err != nil {
			return nil, err
		}
		return plain, nil
	}
	return data, nil
}

func encryptBytes(passphrase, plaintext []byte) ([]byte, error) {
	salt := make([]byte, encryptionSalt)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher gcm: %w", err)
	}

	nonce := make([]byte, encryptionNonce)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	buf := bytes.NewBuffer(make([]byte, 0, len(encryptionMagic)+len(salt)+len(nonce)+len(ciphertext)))
	buf.WriteString(encryptionMagic)
	buf.Write(salt)
	buf.Write(nonce)
	buf.Write(ciphertext)
	return buf.Bytes(), nil
}

func decryptBytes(passphrase, ciphertext []byte) ([]byte, error) {
	header := []byte(encryptionMagic)
	if len(ciphertext) < len(header)+encryptionSalt+encryptionNonce {
		return nil, errors.New("encrypted data truncated")
	}
	if !bytes.Equal(ciphertext[:len(header)], header) {
		return nil, errors.New("invalid encryption magic")
	}
	offset := len(header)
	salt := ciphertext[offset : offset+encryptionSalt]
	offset += encryptionSalt
	nonce := ciphertext[offset : offset+encryptionNonce]
	offset += encryptionNonce
	payload := ciphertext[offset:]

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt pqc keystore: %w", err)
	}
	return plaintext, nil
}

func deriveKey(passphrase, salt []byte) ([]byte, error) {
	key, err := scrypt.Key(passphrase, salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
