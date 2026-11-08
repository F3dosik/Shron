//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusOf возвращает состояние записи name в списке.
func statusOf(t *testing.T, v *vault.Vault, name string) vault.SyncStatus {
	t.Helper()
	list, err := v.ListSecrets(context.Background())
	require.NoError(t, err)
	for _, s := range list {
		if s.Name == name {
			return s.Status
		}
	}
	t.Fatalf("записи %q нет в списке", name)
	return ""
}

func TestE2E_Vault_LocalRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	phone, _, login := newVaultUserAt(t, filepath.Join(dir, "session"), func(cfg *vault.Config) {
		cfg.CachePath = filepath.Join(dir, offline.FileName)
		cfg.OfflineWrites = true
	})
	laptop, _ := openVault(t, filepath.Join(t.TempDir(), "session"), nil)
	_, err := laptop.SignIn(ctx, login, "vault-password-1")
	require.NoError(t, err)

	require.NoError(t, phone.CreateLocalSecret(ctx, credentials("seed", "me", "words", "wallet.example")))
	require.NoError(t, phone.CreateSecret(ctx, credentials("mail", "me", "pw", "mail.example")))
	assert.Equal(t, vault.StatusLocal, statusOf(t, phone, "seed"))
	assert.Equal(t, vault.StatusSynced, statusOf(t, phone, "mail"))
	assert.Equal(t, 1, phone.LocalRecords())

	// Сервер о записи устройства не знает.
	_, err = laptop.GetSecret(ctx, "seed", domain.SecretTypeCredentials)
	require.ErrorIs(t, err, domain.ErrNotFound)
	// Экспорт и автозаполнение её видят.
	exported, err := phone.ExportSecrets(ctx)
	require.NoError(t, err)
	assert.Len(t, exported, 2)
	matches, err := phone.MatchLogins(ctx, "wallet.example", "")
	require.NoError(t, err)
	require.Len(t, matches, 1)

	// Перенос на сервер и обратно.
	require.NoError(t, phone.SetLocal(ctx, "seed", domain.SecretTypeCredentials, false))
	_, err = laptop.GetSecret(ctx, "seed", domain.SecretTypeCredentials)
	require.NoError(t, err)
	require.NoError(t, phone.SetLocal(ctx, "mail", domain.SecretTypeCredentials, true))
	_, err = laptop.GetSecret(ctx, "mail", domain.SecretTypeCredentials)
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, vault.StatusLocal, statusOf(t, phone, "mail"))

	// Смена пароля на этом устройстве перешифровывает записи устройства.
	require.NoError(t, phone.ChangePassword(ctx, "vault-password-1", "vault-password-2", domain.DefaultKDFParams))
	mail, err := phone.GetSecret(ctx, "mail", domain.SecretTypeCredentials)
	require.NoError(t, err)
	assert.Contains(t, string(mail.Data), `"pw"`)
	assert.Equal(t, vault.StatusLocal, statusOf(t, phone, "mail"))

	// Смена пароля на другом устройстве: записи устройства откладываются до ввода
	// прежнего пароля.
	_, err = laptop.SignIn(ctx, login, "vault-password-2")
	require.NoError(t, err)
	require.NoError(t, laptop.ChangePassword(ctx, "vault-password-2", "vault-password-3", domain.DefaultKDFParams))
	_, err = phone.SignIn(ctx, login, "vault-password-3")
	require.NoError(t, err)
	assert.Equal(t, 0, phone.LocalRecords())
	assert.Equal(t, 1, phone.StaleLocalRecords())

	_, err = phone.RecoverLocalRecords(ctx, "wrong-password", "восстановлено")
	require.ErrorIs(t, err, vault.ErrWrongPassword)
	n, err := phone.RecoverLocalRecords(ctx, "vault-password-2", "восстановлено")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, phone.StaleLocalRecords())
	assert.Equal(t, vault.StatusLocal, statusOf(t, phone, "mail"))
	mail, err = phone.GetSecret(ctx, "mail", domain.SecretTypeCredentials)
	require.NoError(t, err)
	assert.Contains(t, string(mail.Data), `"pw"`)

	// Выход из учётки удаляет записи устройства.
	require.NoError(t, phone.Logout(ctx, false))
	_, err = offline.Load(filepath.Join(dir, offline.FileName))
	require.Error(t, err)
}

func TestE2E_Vault_ImportStandalone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	standalonePath := filepath.Join(dir, offline.StandaloneFileName)

	// Хранилище без сервера.
	local, err := vault.Open(vault.Config{Standalone: true, CachePath: standalonePath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = local.Close() })
	require.NoError(t, local.CreateStandalone("standalone-pw", domain.DefaultKDFParams))
	require.NoError(t, local.CreateSecret(ctx, credentials("mail", "me", "pw-local", "mail.example")))
	require.NoError(t, local.CreateSecret(ctx, credentials("seed", "me", "words", "wallet.example")))
	require.NoError(t, local.Close())

	// Аккаунт на сервере, где уже есть запись с тем же названием.
	account, _, _ := newVaultUserAt(t, filepath.Join(dir, "session"), func(cfg *vault.Config) {
		cfg.CachePath = filepath.Join(dir, offline.FileName)
	})
	require.NoError(t, account.CreateSecret(ctx, credentials("mail", "me", "pw-server", "mail.example")))
	assert.Equal(t, 2, account.StandaloneRecords(standalonePath))

	_, err = account.ImportStandalone(ctx, standalonePath, "wrong-password", "с телефона")
	require.ErrorIs(t, err, vault.ErrWrongPassword)
	n, err := account.ImportStandalone(ctx, standalonePath, "standalone-pw", "с телефона")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 0, account.StandaloneRecords(standalonePath))
	_, err = offline.Load(standalonePath)
	require.Error(t, err, "перенесённое хранилище удаляется")

	list, err := account.ListSecrets(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
	seed, err := account.GetSecret(ctx, "seed", domain.SecretTypeCredentials)
	require.NoError(t, err)
	assert.Contains(t, string(seed.Data), "words")
	mail, err := account.GetSecret(ctx, "mail", domain.SecretTypeCredentials)
	require.NoError(t, err)
	assert.Contains(t, string(mail.Data), "pw-server", "запись аккаунта не перезаписана")
	assert.Equal(t, vault.StatusSynced, statusOf(t, account, "seed"))
}
