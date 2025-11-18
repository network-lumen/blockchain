package keys

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()

	store, err := LoadStore(dir)
	require.NoError(t, err)

	record := KeyRecord{
		Name:       "test",
		Scheme:     "dilithium3",
		PublicKey:  []byte{1, 2, 3},
		PrivateKey: []byte{4, 5, 6},
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, store.SaveKey(record))
	require.NoError(t, store.LinkAddress("addr1", "test"))

	reloaded, err := LoadStore(dir)
	require.NoError(t, err)

	key, ok := reloaded.GetKey("test")
	require.True(t, ok)
	require.Equal(t, record.Name, key.Name)
	require.Equal(t, record.Scheme, key.Scheme)
	require.Equal(t, record.PublicKey, key.PublicKey)
	require.Equal(t, record.PrivateKey, key.PrivateKey)

	link, ok := reloaded.GetLink("addr1")
	require.True(t, ok)
	require.Equal(t, "test", link)

	// files created
	_, err = os.Stat(filepath.Join(dir, storageDirName, keysFileName))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, storageDirName, linksFileName))
	require.NoError(t, err)
}

func TestStoreWithPassphrase(t *testing.T) {
	dir := t.TempDir()
	pass := []byte("s3cret-passphrase")

	store, err := LoadStore(dir, WithPassphrase(pass))
	require.NoError(t, err)

	record := KeyRecord{
		Name:       "enc",
		Scheme:     "dilithium3",
		PublicKey:  []byte{9, 9, 9},
		PrivateKey: []byte{8, 8, 8},
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.SaveKey(record))
	require.NoError(t, store.LinkAddress("addr-enc", "enc"))

	reloaded, err := LoadStore(dir, WithPassphrase(pass))
	require.NoError(t, err)

	key, ok := reloaded.GetKey("enc")
	require.True(t, ok)
	require.Equal(t, record.PublicKey, key.PublicKey)

	// ensure ciphertext stored on disk
	data, err := os.ReadFile(filepath.Join(dir, storageDirName, keysFileName))
	require.NoError(t, err)
	require.Contains(t, string(data[:8]), "PQCENC1")

	// missing passphrase should error
	_, err = LoadStore(dir)
	require.Error(t, err)
}
