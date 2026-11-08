package offline_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSnapshot() *offline.Snapshot {
	return &offline.Snapshot{
		Server:   "vault.example:8443",
		Login:    "alice",
		Salt:     []byte("0123456789abcdef"),
		KDF:      domain.DefaultKDFParams,
		SyncedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		Records: []offline.Record{
			{BlindIndex: "bi-1", Data: []byte("ciphertext-1"), CreatedAt: time.Unix(100, 0).UTC(), UpdatedAt: time.Unix(200, 0).UTC()},
		},
	}
}

func TestSaveLoadOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), offline.FileName)
	key := []byte("master-key-master-key-master-key!")
	require.NoError(t, offline.Save(path, key, testSnapshot()))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	sealed, err := offline.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "alice", sealed.Unverified().Login)

	snap, err := sealed.Open(key)
	require.NoError(t, err)
	assert.Equal(t, testSnapshot(), snap)

	_, err = sealed.Open([]byte("another-key-another-key-another!!"))
	assert.ErrorIs(t, err, offline.ErrWrongKey)
}

func TestOpenDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), offline.FileName)
	key := []byte("master-key-master-key-master-key!")
	require.NoError(t, offline.Save(path, key, testSnapshot()))

	// Подменяем соль в теле, оставляя подпись прежней.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var f map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &f))
	var body map[string]any
	require.NoError(t, json.Unmarshal(f["body"], &body))
	body["salt"] = []byte("weaker-salt-weak")
	f["body"], err = json.Marshal(body)
	require.NoError(t, err)
	data, err = json.Marshal(f)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	sealed, err := offline.Load(path)
	require.NoError(t, err)
	_, err = sealed.Open(key)
	assert.ErrorIs(t, err, offline.ErrWrongKey)
}

func TestLoadMissingAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), offline.FileName)
	_, err := offline.Load(path)
	assert.True(t, errors.Is(err, os.ErrNotExist))
	assert.NoError(t, offline.Remove(path))

	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
	_, err = offline.Load(path)
	assert.Error(t, err)
	assert.NoError(t, offline.Remove(path))
	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}
