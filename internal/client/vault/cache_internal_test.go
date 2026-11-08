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
	if _, ok := f.secrets[bi]; ok {
		return domain.ErrAlreadyExists
	}
	f.secrets[bi] = &domain.Secret{BlindIndex: bi, Data: data}
	return nil
}

func (f *fakeRemote) UpdateSecret(_ context.Context, bi string, data []byte) error {
	if err := f.check(); err != nil {
		return err
	}
	if _, ok := f.secrets[bi]; !ok {
		return domain.ErrNotFound
	}
	f.secrets[bi] = &domain.Secret{BlindIndex: bi, Data: data}
	return nil
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
	c := newCachedClient(remote, filepath.Join(t.TempDir(), offline.FileName), 30*time.Second, true)
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

func TestCachedClientQueuesOfflineChanges(t *testing.T) {
	ctx := context.Background()
	c, remote, now := newTestCache(t)
	require.NoError(t, c.CreateSecret(ctx, "edit", []byte("edit-v1")))
	require.NoError(t, c.CreateSecret(ctx, "drop", []byte("drop-v1")))
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)

	remote.down = true
	require.NoError(t, c.UpdateSecret(ctx, "edit", []byte("edit-v2")))
	require.NoError(t, c.DeleteSecret(ctx, "drop"))
	require.NoError(t, c.CreateSecret(ctx, "new", []byte("new-v1")))
	// Создано и удалено без сети — отправлять нечего.
	require.NoError(t, c.CreateSecret(ctx, "tmp", []byte("tmp")))
	require.NoError(t, c.DeleteSecret(ctx, "tmp"))
	assert.ErrorIs(t, c.CreateSecret(ctx, "edit", []byte("x")), domain.ErrAlreadyExists)
	assert.ErrorIs(t, c.UpdateSecret(ctx, "missing", []byte("x")), domain.ErrNotFound)
	require.Len(t, c.pendingOps(), 3)

	// Список и чтение — с учётом очереди.
	list, err := c.ListSecrets(ctx)
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range list {
		got[s.BlindIndex] = string(s.Data)
	}
	assert.Equal(t, map[string]string{"edit": "edit-v2", "new": "new-v1"}, got)

	// Очередь переживает перезапуск: она в подписанной копии.
	sealed, err := offline.Load(c.path)
	require.NoError(t, err)
	snap, err := sealed.Open([]byte("key"))
	require.NoError(t, err)
	assert.Len(t, snap.Pending, 3)

	remote.down = false
	*now = now.Add(time.Minute)
	require.NoError(t, c.sync(ctx))
	assert.Empty(t, c.pendingOps())
	assert.Equal(t, []byte("edit-v2"), remote.secrets["edit"].Data)
	assert.Equal(t, []byte("new-v1"), remote.secrets["new"].Data)
	assert.NotContains(t, remote.secrets, "drop")
}

func TestCachedClientConflicts(t *testing.T) {
	ctx := context.Background()
	c, remote, now := newTestCache(t)
	for _, bi := range []string{"changed", "gone", "deleted-here"} {
		require.NoError(t, c.CreateSecret(ctx, bi, []byte(bi+"-v1")))
	}
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)

	remote.down = true
	require.NoError(t, c.UpdateSecret(ctx, "changed", []byte("changed-phone")))
	require.NoError(t, c.UpdateSecret(ctx, "gone", []byte("gone-phone")))
	require.NoError(t, c.DeleteSecret(ctx, "deleted-here"))
	require.NoError(t, c.CreateSecret(ctx, "dup", []byte("dup-phone")))

	// Тем временем на другом устройстве.
	remote.down = false
	remote.secrets["changed"] = &domain.Secret{BlindIndex: "changed", Data: []byte("changed-other"), UpdatedAt: *now}
	delete(remote.secrets, "gone")
	remote.secrets["deleted-here"] = &domain.Secret{BlindIndex: "deleted-here", Data: []byte("deleted-other")}
	remote.secrets["dup"] = &domain.Secret{BlindIndex: "dup", Data: []byte("dup-other")}

	*now = now.Add(time.Minute)
	require.NoError(t, c.sync(ctx))
	kinds := map[string]offline.ConflictKind{}
	for _, op := range c.pendingOps() {
		require.NotNil(t, op.Conflict)
		kinds[op.BlindIndex] = op.Conflict.Kind
	}
	assert.Equal(t, map[string]offline.ConflictKind{
		"changed":      offline.ConflictChanged,
		"gone":         offline.ConflictDeletedOnServer,
		"deleted-here": offline.ConflictChangedOnServer,
		"dup":          offline.ConflictExists,
	}, kinds)
	// Сервер не тронут.
	assert.Equal(t, []byte("changed-other"), remote.secrets["changed"].Data)

	// Решения.
	require.NoError(t, c.resolve("changed", ResolveMine))
	require.NoError(t, c.resolve("gone", ResolveMine))
	require.NoError(t, c.resolve("deleted-here", ResolveServer))
	require.NoError(t, c.resolve("dup", ResolveMine))
	assert.ErrorIs(t, c.resolve("dup", ResolveMine), ErrNoConflict)
	require.NoError(t, c.sync(ctx))
	assert.Empty(t, c.pendingOps())
	assert.Equal(t, []byte("changed-phone"), remote.secrets["changed"].Data)
	assert.Equal(t, []byte("gone-phone"), remote.secrets["gone"].Data)
	assert.Equal(t, []byte("deleted-other"), remote.secrets["deleted-here"].Data)
	assert.Equal(t, []byte("dup-phone"), remote.secrets["dup"].Data)
}

func TestCachedClientMineConflictsAgainIfChangedMeanwhile(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	require.NoError(t, c.CreateSecret(ctx, "bi", []byte("v1")))
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)

	remote.down = true
	require.NoError(t, c.UpdateSecret(ctx, "bi", []byte("phone")))
	remote.down = false
	c.retryAt = time.Time{}
	remote.secrets["bi"] = &domain.Secret{BlindIndex: "bi", Data: []byte("other-1")}
	require.NoError(t, c.sync(ctx))
	require.NoError(t, c.resolve("bi", ResolveMine))

	// Пока пользователь думал, запись изменили ещё раз.
	remote.secrets["bi"] = &domain.Secret{BlindIndex: "bi", Data: []byte("other-2")}
	require.NoError(t, c.sync(ctx))
	ops := c.pendingOps()
	require.Len(t, ops, 1)
	require.NotNil(t, ops[0].Conflict)
	assert.Equal(t, []byte("other-2"), ops[0].Conflict.ServerData)
	assert.Equal(t, []byte("other-2"), remote.secrets["bi"].Data)
}

func TestCachedClientReadOnlyWithoutQueue(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	c.queue = false
	require.NoError(t, c.CreateSecret(ctx, "bi", []byte("v1")))
	_, err := c.ListSecrets(ctx)
	require.NoError(t, err)

	remote.down = true
	assert.ErrorIs(t, c.UpdateSecret(ctx, "bi", []byte("v2")), domain.ErrUnavailable)
	assert.Empty(t, c.pendingOps())
}
