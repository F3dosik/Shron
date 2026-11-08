//go:build e2e

package e2e

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/mobile"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tcpProxy пересылает соединения на тестовый сервер; Stop имитирует пропавшую сеть,
// Start — её возвращение на том же адресе.
type tcpProxy struct {
	t      *testing.T
	addr   string
	target string

	mu    sync.Mutex
	ln    net.Listener
	conns []net.Conn
}

func startProxy(t *testing.T, target string) *tcpProxy {
	t.Helper()
	p := &tcpProxy{t: t, addr: "127.0.0.1:0", target: target}
	p.Start()
	t.Cleanup(p.Stop)
	return p
}

func (p *tcpProxy) Start() {
	p.t.Helper()
	ln, err := net.Listen("tcp", p.addr)
	require.NoError(p.t, err)
	p.mu.Lock()
	p.ln, p.addr = ln, ln.Addr().String()
	p.mu.Unlock()
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", p.target)
			if err != nil {
				client.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, client, server)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(server, client); server.Close() }()
			go func() { _, _ = io.Copy(client, server); client.Close() }()
		}
	}()
}

func (p *tcpProxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ln != nil {
		p.ln.Close()
		p.ln = nil
	}
	for _, c := range p.conns {
		c.Close()
	}
	p.conns = nil
}

func credentials(name, login, password, uri string) *domain.SecretPayload {
	return &domain.SecretPayload{
		Name: name, Type: domain.SecretTypeCredentials,
		Data: []byte(`{"login":"` + login + `","password":"` + password + `","uris":["` + uri + `"]}`),
	}
}

func TestE2E_Vault_Offline(t *testing.T) {
	ctx := context.Background()
	proxy := startProxy(t, serverAddr)
	dir := t.TempDir()
	cachePath := filepath.Join(dir, offline.FileName)
	configure := func(cfg *vault.Config) {
		cfg.ServerAddress = proxy.addr
		cfg.CachePath = cachePath
		cfg.OfflineRetryAfter = 100 * time.Millisecond
	}
	v, _, login := newVaultUserAt(t, filepath.Join(dir, "session"), configure)

	require.NoError(t, v.CreateSecret(ctx, credentials("github", "me", "pw-1", "github.com")))
	_, err := v.ListSecrets(ctx)
	require.NoError(t, err)
	_, err = os.Stat(cachePath)
	require.NoError(t, err, "копия должна сохраниться после списка")
	masterKey, err := v.MasterKey()
	require.NoError(t, err)
	v.Lock()
	require.NoError(t, v.Close())

	// Приложение запускается без сети.
	proxy.Stop()
	v, locks := openVault(t, filepath.Join(dir, "session"), configure)
	assert.Equal(t, login, v.CurrentLogin())

	err = v.Unlock(ctx, "wrong-password-1")
	assert.Equal(t, vault.CodeWrongPassword, vault.ErrorCode(err))
	require.NoError(t, v.Unlock(ctx, "vault-password-1"))
	offlineNow, syncedAt := v.Offline()
	assert.True(t, offlineNow)
	assert.False(t, syncedAt.IsZero())

	list, err := v.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "github", list[0].Name)
	matches, err := v.MatchLogins(ctx, "github.com", "")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "pw-1", matches[0].Password)
	info, err := v.GetSecret(ctx, "github", domain.SecretTypeCredentials)
	require.NoError(t, err)
	assert.Equal(t, "github", info.Name)

	// Без сервера — только чтение.
	time.Sleep(150 * time.Millisecond)
	err = v.CreateSecret(ctx, credentials("gitlab", "me", "pw-2", "gitlab.com"))
	assert.Equal(t, vault.CodeUnavailable, vault.ErrorCode(err))
	assert.True(t, v.IsUnlocked(), "сбой сети не блокирует хранилище")

	// Мастер-ключ (биометрия) без сервера проверяется по копии.
	v.Lock()
	err = v.UnlockWithKey(ctx, []byte("0123456789abcdef0123456789abcdef"))
	assert.Equal(t, vault.CodeKeyRejected, vault.ErrorCode(err))
	require.NoError(t, v.UnlockWithKey(ctx, masterKey))
	offlineNow, _ = v.Offline()
	assert.True(t, offlineNow)

	// Сеть вернулась: вход по мастер-ключу, затем можно менять записи.
	proxy.Start()
	require.Eventually(t, func() bool {
		_, err := v.ListSecrets(ctx)
		offlineNow, _ := v.Offline()
		return err == nil && !offlineNow
	}, 30*time.Second, 200*time.Millisecond)
	require.NoError(t, v.CreateSecret(ctx, credentials("gitlab", "me", "pw-2", "gitlab.com")))
	assert.Equal(t, vault.LockManual, locks.last(), "после возврата сети хранилище не блокируется")
	assert.True(t, v.IsUnlocked())

	// Выход удаляет копию.
	require.NoError(t, v.Logout(ctx, false))
	_, err = os.Stat(cachePath)
	assert.True(t, os.IsNotExist(err))
}

func TestE2E_Vault_OfflineNoCopy(t *testing.T) {
	ctx := context.Background()
	proxy := startProxy(t, serverAddr)
	dir := t.TempDir()
	configure := func(cfg *vault.Config) { cfg.ServerAddress = proxy.addr }
	v, _, _ := newVaultUserAt(t, filepath.Join(dir, "session"), configure)
	_, err := v.ListSecrets(ctx)
	require.NoError(t, err)
	v.Lock()

	proxy.Stop()
	err = v.Unlock(ctx, "vault-password-1")
	assert.Equal(t, vault.CodeUnavailable, vault.ErrorCode(err))
	assert.False(t, v.IsUnlocked())
}

func TestE2E_Mobile_Offline(t *testing.T) {
	proxy := startProxy(t, serverAddr)
	dir := t.TempDir()
	app := newMobileApp(t, dir, nil)
	require.NoError(t, app.SaveSettings(proxy.addr, "", false, 5))

	login := "mob-off-" + uuid.NewString()
	require.NoError(t, app.Register(login, "mobile-pass-1"))
	_, err := app.SignIn(login, "mobile-pass-1")
	require.NoError(t, err)
	require.NoError(t, app.SaveSecret(&mobile.SecretInput{
		Name: "bank", Type: mobile.TypeCredentials, Login: "me", Password: "pw-1", URIs: "androidapp://com.bank.app",
	}, false))
	_, err = app.ListSecrets()
	require.NoError(t, err)
	assert.NotZero(t, app.GetState().SyncedAt)
	app.Close()

	proxy.Stop()
	app = newMobileApp(t, dir, nil)
	require.NoError(t, app.Unlock("mobile-pass-1"))
	state := app.GetState()
	assert.True(t, state.Unlocked)
	assert.True(t, state.Offline)

	matches, err := app.MatchLogins("", "com.bank.app")
	require.NoError(t, err)
	require.Equal(t, 1, matches.Len())
	assert.Equal(t, "pw-1", matches.Get(0).Password)

	err = app.SaveSecret(&mobile.SecretInput{Name: "new", Type: mobile.TypeText, Text: "x"}, false)
	assert.Equal(t, mobile.CodeUnavailable, mobileCode(t, err))
}
