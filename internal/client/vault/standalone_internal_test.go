package vault

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastKDF — слабые, но допустимые параметры Argon2id, чтобы тесты шли быстро.
var fastKDF = domain.KDFParams{Time: 1, MemoryKiB: 19 * 1024, Threads: 1}

func openStandalone(t *testing.T) (*Vault, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), offline.StandaloneFileName)
	v, err := Open(Config{Standalone: true, CachePath: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	return v, path
}

func payload(name, data string) *domain.SecretPayload {
	return &domain.SecretPayload{Name: name, Type: domain.SecretTypeText, Data: []byte(`{"text":"` + data + `"}`)}
}

func TestStandaloneVault(t *testing.T) {
	ctx := context.Background()
	v, path := openStandalone(t)
	assert.False(t, v.HasStandaloneVault())
	require.ErrorIs(t, v.Unlock(ctx, "password-1"), ErrNotSignedIn)

	require.NoError(t, v.CreateStandalone("password-1", fastKDF))
	assert.True(t, v.HasStandaloneVault())
	assert.True(t, v.IsUnlocked())
	require.ErrorIs(t, v.CreateStandalone("password-1", fastKDF), domain.ErrAlreadyExists)

	require.NoError(t, v.CreateSecret(ctx, payload("note", "one")))
	require.NoError(t, v.UpdateSecret(ctx, payload("note", "two")))
	require.NoError(t, v.CreateSecret(ctx, payload("other", "x")))
	require.NoError(t, v.DeleteSecret(ctx, "other", domain.SecretTypeText))
	list, err := v.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, StatusSynced, list[0].Status)

	// Серверные операции недоступны.
	require.ErrorIs(t, v.Logout(ctx, false), ErrStandalone)
	require.ErrorIs(t, v.SetLocal(ctx, "note", domain.SecretTypeText, false), ErrStandalone)
	_, err = v.SignIn(ctx, "alice", "password-1")
	require.ErrorIs(t, err, ErrStandalone)

	// Разблокировка паролем и ключом.
	key, err := v.MasterKey()
	require.NoError(t, err)
	v.Lock()
	require.ErrorIs(t, v.Unlock(ctx, "wrong-password"), ErrWrongPassword)
	require.NoError(t, v.Unlock(ctx, "password-1"))
	v.Lock()
	require.NoError(t, v.UnlockWithKey(ctx, key))
	got, err := v.GetSecret(ctx, "note", domain.SecretTypeText)
	require.NoError(t, err)
	assert.Contains(t, string(got.Data), "two")

	// Смена пароля.
	require.ErrorIs(t, v.ChangePassword(ctx, "wrong-password", "password-2", fastKDF), ErrWrongPassword)
	require.NoError(t, v.ChangePassword(ctx, "password-1", "password-2", fastKDF))
	v.Lock()
	require.ErrorIs(t, v.Unlock(ctx, "password-1"), ErrWrongPassword)
	require.ErrorIs(t, v.UnlockWithKey(ctx, key), ErrKeyRejected)
	require.NoError(t, v.Unlock(ctx, "password-2"))
	got, err = v.GetSecret(ctx, "note", domain.SecretTypeText)
	require.NoError(t, err)
	assert.Contains(t, string(got.Data), "two")

	// Удаление хранилища.
	require.ErrorIs(t, v.DeleteAccount(ctx, "password-1"), ErrWrongPassword)
	require.NoError(t, v.DeleteAccount(ctx, "password-2"))
	assert.False(t, v.IsUnlocked())
	assert.False(t, v.HasStandaloneVault())
	_, err = offline.Load(path)
	require.Error(t, err)
}
