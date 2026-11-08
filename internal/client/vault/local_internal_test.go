package vault

import (
	"context"
	"testing"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedClientLocalRecords(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	require.NoError(t, c.CreateSecret(ctx, "server", []byte("s")))

	// Запись только на устройстве не уходит на сервер, но видна в списке и по запросу.
	require.NoError(t, c.CreateSecret(withLocal(ctx), "local", []byte("l")))
	assert.NotContains(t, remote.secrets, "local")
	list, err := c.ListSecrets(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	calls := remote.calls
	got, err := c.GetSecret(ctx, "local")
	require.NoError(t, err)
	assert.Equal(t, []byte("l"), got.Data)
	assert.Equal(t, calls, remote.calls, "запись устройства не запрашивается с сервера")

	// Названия не повторяются ни на сервере, ни на устройстве.
	require.ErrorIs(t, c.CreateSecret(withLocal(ctx), "server", []byte("x")), domain.ErrAlreadyExists)
	require.ErrorIs(t, c.CreateSecret(ctx, "local", []byte("x")), domain.ErrAlreadyExists)

	// Изменение и удаление — только на устройстве, и без сервера тоже.
	remote.down = true
	require.NoError(t, c.UpdateSecret(ctx, "local", []byte("l2")))
	assert.Empty(t, c.pendingOps())
	got, err = c.GetSecret(ctx, "local")
	require.NoError(t, err)
	assert.Equal(t, []byte("l2"), got.Data)

	// Записи устройства сохранены в файле копии.
	sealed, err := offline.Load(c.path)
	require.NoError(t, err)
	snap, err := sealed.Open([]byte("key"))
	require.NoError(t, err)
	require.Len(t, snap.Local, 1)
	assert.Equal(t, []byte("l2"), snap.Local[0].Data)

	require.NoError(t, c.DeleteSecret(ctx, "local"))
	assert.Equal(t, 0, c.localCount())
	assert.Empty(t, c.pendingOps())
}

func TestCachedClientMoveRecords(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	require.NoError(t, c.CreateSecret(ctx, "bi", []byte("data")))

	// На устройство: запись удаляется с сервера и остаётся на устройстве.
	require.NoError(t, c.moveToDevice(ctx, "bi"))
	assert.NotContains(t, remote.secrets, "bi")
	assert.Equal(t, 1, c.localCount())
	list, err := c.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Обратно на сервер.
	require.NoError(t, c.moveToServer(ctx, "bi"))
	assert.Equal(t, []byte("data"), remote.secrets["bi"].Data)
	assert.Equal(t, 0, c.localCount())

	// Без сервера перенос на устройство ставит удаление в очередь.
	_, err = c.ListSecrets(ctx)
	require.NoError(t, err)
	remote.down = true
	require.NoError(t, c.moveToDevice(ctx, "bi"))
	assert.Equal(t, 1, c.localCount())
	require.Len(t, c.pendingOps(), 1)
	list, err = c.ListSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "запись сервера, ждущая удаления, не показывается рядом с записью устройства")

	remote.down = false
	c.online()
	require.NoError(t, c.sync(ctx))
	assert.NotContains(t, remote.secrets, "bi")
	assert.Empty(t, c.pendingOps())
	assert.Equal(t, 1, c.localCount())
}

func TestCachedClientMoveToServerKeepsRecordOnError(t *testing.T) {
	ctx := context.Background()
	c, remote, _ := newTestCache(t)
	require.NoError(t, c.CreateSecret(withLocal(ctx), "bi", []byte("mine")))
	// Запись с тем же названием создали на сервере с другого устройства.
	remote.secrets["bi"] = &domain.Secret{BlindIndex: "bi", Data: []byte("theirs")}

	require.ErrorIs(t, c.moveToServer(ctx, "bi"), domain.ErrAlreadyExists)
	assert.Equal(t, 1, c.localCount())
	got, err := c.GetSecret(ctx, "bi")
	require.NoError(t, err)
	assert.Equal(t, []byte("mine"), got.Data)
}

func TestCachedClientLocalNeedsCopy(t *testing.T) {
	c := newCachedClient(newFakeRemote(), "", offlineRetryAfter, false)
	require.ErrorIs(t, c.CreateSecret(withLocal(context.Background()), "bi", []byte("x")), ErrLocalUnavailable)
	assert.Equal(t, CodeNotSupported, ErrorCode(ErrLocalUnavailable))
}
