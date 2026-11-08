package vault

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRemote — сервер секретов в памяти, который можно «отключить».
type fakeRemote struct {
	down    bool
	secrets map[string]*domain.Secret
	calls   int
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{secrets: map[string]*domain.Secret{}}
}

var errNetwork = fmt.Errorf("%w: connection refused", domain.ErrUnavailable)

func (f *fakeRemote) check() error {
	f.calls++
	if f.down {
		return errNetwork
	}
	return nil
}

func (f *fakeRemote) ListSecrets(context.Context) ([]*domain.Secret, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	var result []*domain.Secret
	for _, s := range f.secrets {
		result = append(result, s)
	}
	return result, nil
}

func (f *fakeRemote) CreateSecret(_ context.Context, bi string, data []byte) error {
	if err := f.check(); err != nil {
		return err
	}
	f.secrets[bi] = &domain.Secret{BlindIndex: bi, Data: data}
	return nil
}

func (f *fakeRemote) UpdateSecret(ctx context.Context, bi string, data []byte) error {
	return f.CreateSecret(ctx, bi, data)
}

func (f *fakeRemote) GetSecret(_ context.Context, bi string) (*domain.Secret, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	if s, ok := f.secrets[bi]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeRemote) DeleteSecret(_ context.Context, bi string) error {
	if err := f.check(); err != nil {
		return err
	}
	delete(f.secrets, bi)
	return nil
}

func newTestCache(t *testing.T) (*cachedClient, *fakeRemote, *time.Time) {
	t.Helper()
	remote := newFakeRemote()
	c := newCachedClient(remote, filepath.Join(t.TempDir(), offline.FileName), 30*time.Second)
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	c.attach(&offline.Snapshot{Server: "s", Login: "alice"}, []byte("key"), false)
	return c, remote, &now
}

func TestCachedClientFallsBackWhenServerDown(t *testing.T) {
	ctx := context.Background()
	c, remote, now := newTestCache(t)

	require.NoError(t, c.CreateSecret(ctx, "bi-1", []byte("one")))
	list, err := c.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.False(t, c.isOffline())
	assert.Equal(t, *now, c.syncedAt())

	// Копия записана на диск и подписана ключом.
	sealed, err := offline.Load(c.path)
	require.NoError(t, err)
	snap, err := sealed.Open([]byte("key"))
	require.NoError(t, err)
	require.Len(t, snap.Records, 1)

	remote.down = true
	list, err = c.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, []byte("one"), list[0].Data)
	assert.True(t, c.isOffline())

	got, err := c.GetSecret(ctx, "bi-1")
	require.NoError(t, err)
	assert.Equal(t, []byte("one"), got.Data)
	_, err = c.GetSecret(ctx, "missing")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Изменения без сервера невозможны.
	err = c.CreateSecret(ctx, "bi-2", []byte("two"))
	assert.ErrorIs(t, err, domain.ErrUnavailable)

	// В окне повтора чтение не ждёт сервер.
	calls := remote.calls
	_, err = c.ListSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, calls, remote.calls)

	// После окна — снова сервер.
	remote.down = false
	*now = now.Add(time.Minute)
	_, err = c.ListSecrets(ctx)
	require.NoError(t, err)
	assert.False(t, c.isOffline())
	assert.Equal(t, calls+1, remote.calls)
}

func TestCachedClientNoFallback(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)
	remote.down = true

	// Перешифровке копия не годится.
	_, err = c.ListSecrets(withoutCache(ctx))
	assert.ErrorIs(t, err, domain.ErrUnavailable)

	// Ошибка сертификата — не сбой сети.
	remote.down = false
	certErr := fmt.Errorf("%w: x509: certificate signed by unknown authority", domain.ErrUnavailable)
	assert.False(t, c.fallback(ctx, certErr))

	// Без копии (заблокировано) — ошибка как есть.
	c.detach()
	remote.down = true
	_, err = c.ListSecrets(ctx)
	assert.ErrorIs(t, err, domain.ErrUnavailable)
}

func TestCachedClientWritesUpdateCopy(t *testing.T) {
	ctx := context.Background()
	c, _, _ := newTestCache(t)
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)

	require.NoError(t, c.CreateSecret(ctx, "bi-1", []byte("one")))
	require.NoError(t, c.UpdateSecret(ctx, "bi-1", []byte("one-v2")))
	require.NoError(t, c.CreateSecret(ctx, "bi-2", []byte("two")))
	require.NoError(t, c.DeleteSecret(ctx, "bi-2"))

	sealed, err := offline.Load(c.path)
	require.NoError(t, err)
	snap, err := sealed.Open([]byte("key"))
	require.NoError(t, err)
	require.Len(t, snap.Records, 1)
	assert.Equal(t, []byte("one-v2"), snap.Records[0].Data)
}
