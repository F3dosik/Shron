package vault

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
)

const (
	// offlineReadTimeout — сколько ждать сервер при чтении, когда есть копия на
	// устройстве: дольше пользователь (и автозаполнение) ждать не станет.
	offlineReadTimeout = 10 * time.Second
	// offlineRetryAfter — по умолчанию сколько после неудачи читать из копии, не обращаясь к серверу:
	// иначе без сети каждое действие ждало бы offlineReadTimeout.
	offlineRetryAfter = 30 * time.Second
)

// cachedClient — SecretsClient, который держит копию записей на устройстве
// (offline.Snapshot): обновляет её после каждого ответа сервера и отдаёт записи из неё,
// когда сервер недоступен. Изменения без сервера невозможны: копия только для чтения.
//
// Все методы вызываются под Vault.mu, поэтому своей блокировки у cachedClient нет.
type cachedClient struct {
	remote grpcclient.SecretsClient
	// path — файл копии; пустой — копия выключена, клиент только передаёт запросы.
	path string
	now  func() time.Time
	// retryAfter — сколько после сбоя сети читать из копии, не обращаясь к серверу.
	retryAfter time.Duration

	// snap — проверенная копия разблокированной учётки; nil — копии нет (хранилище
	// заблокировано или копия выключена).
	snap *offline.Snapshot
	// key — мастер-ключ для подписи копии (своя копия ключа, стирается в detach).
	key []byte
	// offline — последний запрос к серверу не прошёл из-за сети, записи — из копии.
	offline bool
	// retryAt — до этого времени чтение идёт из копии без обращения к серверу.
	retryAt time.Time
}

// noCacheKey помечает контекст операций, которым копия не годится (перешифровка при
// смене пароля должна видеть ровно то, что на сервере).
type noCacheKey struct{}

func withoutCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, noCacheKey{}, true)
}

func newCachedClient(remote grpcclient.SecretsClient, path string, retryAfter time.Duration) *cachedClient {
	return &cachedClient{remote: remote, path: path, now: time.Now, retryAfter: retryAfter}
}

// attach подключает проверенную копию разблокированной учётки. offlineNow — хранилище
// разблокировано без сервера: записи сразу читаются из копии.
func (c *cachedClient) attach(snap *offline.Snapshot, masterKey []byte, offlineNow bool) {
	c.detach()
	if c.path == "" {
		return
	}
	c.snap = snap
	c.key = append([]byte(nil), masterKey...)
	if offlineNow {
		c.markOffline()
	}
}

// detach отключает копию и стирает ключ. Файл копии остаётся.
func (c *cachedClient) detach() {
	clear(c.key)
	c.key = nil
	c.snap = nil
	c.offline = false
	c.retryAt = time.Time{}
}

// isOffline сообщает, что записи сейчас читаются из копии.
func (c *cachedClient) isOffline() bool {
	return c.snap != nil && c.offline
}

// syncedAt — когда копия последний раз получила записи с сервера.
func (c *cachedClient) syncedAt() time.Time {
	if c.snap == nil {
		return time.Time{}
	}
	return c.snap.SyncedAt
}

// skipRemote — сервер недавно не ответил: читать из копии, не дожидаясь его.
func (c *cachedClient) skipRemote(ctx context.Context) bool {
	return c.isOffline() && c.now().Before(c.retryAt) && !noCache(ctx)
}

func (c *cachedClient) markOffline() {
	c.offline = true
	c.retryAt = c.now().Add(c.retryAfter)
}

// fallback решает по ошибке сервера, отдавать ли записи из копии: только при сбое сети.
// Ошибка сертификата — не сбой сети, а возможная подмена сервера: о ней нужно сказать.
func (c *cachedClient) fallback(ctx context.Context, err error) bool {
	if c.snap == nil || noCache(ctx) || !isNetworkError(err) {
		return false
	}
	c.markOffline()
	return true
}

// online отмечает успешный ответ сервера.
func (c *cachedClient) online() {
	c.offline = false
	c.retryAt = time.Time{}
}

// readContext ограничивает ожидание сервера при чтении, если есть копия.
func (c *cachedClient) readContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.snap == nil || noCache(ctx) {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, offlineReadTimeout)
}

func (c *cachedClient) ListSecrets(ctx context.Context) ([]*domain.Secret, error) {
	if c.skipRemote(ctx) {
		return c.cachedSecrets(), nil
	}
	rctx, cancel := c.readContext(ctx)
	secrets, err := c.remote.ListSecrets(rctx)
	cancel()
	if err != nil {
		if c.fallback(ctx, err) {
			return c.cachedSecrets(), nil
		}
		return nil, err
	}
	c.online()
	if c.snap != nil {
		c.snap.Records = make([]offline.Record, 0, len(secrets))
		for _, s := range secrets {
			c.snap.Records = append(c.snap.Records, offline.Record{
				BlindIndex: s.BlindIndex, Data: s.Data, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
			})
		}
		c.snap.SyncedAt = c.now()
		c.save()
	}
	return secrets, nil
}

func (c *cachedClient) GetSecret(ctx context.Context, blindIndex string) (*domain.Secret, error) {
	if c.skipRemote(ctx) {
		return c.cachedSecret(blindIndex)
	}
	rctx, cancel := c.readContext(ctx)
	secret, err := c.remote.GetSecret(rctx, blindIndex)
	cancel()
	if err != nil {
		if c.fallback(ctx, err) {
			return c.cachedSecret(blindIndex)
		}
		return nil, err
	}
	c.online()
	// Копия на диске обновится при следующем списке; в памяти — сразу.
	c.put(secret.BlindIndex, secret.Data, secret.CreatedAt, secret.UpdatedAt)
	return secret, nil
}

func (c *cachedClient) CreateSecret(ctx context.Context, blindIndex string, data []byte) error {
	if err := c.write(c.remote.CreateSecret(ctx, blindIndex, data)); err != nil {
		return err
	}
	now := c.now()
	c.put(blindIndex, data, now, now)
	c.save()
	return nil
}

func (c *cachedClient) UpdateSecret(ctx context.Context, blindIndex string, data []byte) error {
	if err := c.write(c.remote.UpdateSecret(ctx, blindIndex, data)); err != nil {
		return err
	}
	createdAt := c.now()
	if i := c.find(blindIndex); i >= 0 {
		createdAt = c.snap.Records[i].CreatedAt
	}
	c.put(blindIndex, data, createdAt, c.now())
	c.save()
	return nil
}

func (c *cachedClient) DeleteSecret(ctx context.Context, blindIndex string) error {
	if err := c.write(c.remote.DeleteSecret(ctx, blindIndex)); err != nil {
		return err
	}
	if i := c.find(blindIndex); i >= 0 {
		c.snap.Records = slices.Delete(c.snap.Records, i, i+1)
		c.save()
	}
	return nil
}

// write отмечает результат изменения на сервере. Изменения всегда идут на сервер,
// даже когда чтение — из копии: так запись сразу проходит, как только сеть вернулась.
func (c *cachedClient) write(err error) error {
	switch {
	case err == nil:
		c.online()
	case c.snap != nil && isNetworkError(err):
		c.markOffline()
	}
	return err
}

func (c *cachedClient) cachedSecrets() []*domain.Secret {
	result := make([]*domain.Secret, 0, len(c.snap.Records))
	for _, r := range c.snap.Records {
		result = append(result, toSecret(r))
	}
	return result
}

func (c *cachedClient) cachedSecret(blindIndex string) (*domain.Secret, error) {
	if i := c.find(blindIndex); i >= 0 {
		return toSecret(c.snap.Records[i]), nil
	}
	return nil, domain.ErrNotFound
}

func (c *cachedClient) find(blindIndex string) int {
	if c.snap == nil {
		return -1
	}
	return slices.IndexFunc(c.snap.Records, func(r offline.Record) bool { return r.BlindIndex == blindIndex })
}

// put добавляет или заменяет запись в копии в памяти.
func (c *cachedClient) put(blindIndex string, data []byte, createdAt, updatedAt time.Time) {
	if c.snap == nil {
		return
	}
	r := offline.Record{BlindIndex: blindIndex, Data: data, CreatedAt: createdAt, UpdatedAt: updatedAt}
	if i := c.find(blindIndex); i >= 0 {
		c.snap.Records[i] = r
		return
	}
	c.snap.Records = append(c.snap.Records, r)
}

// save записывает копию на диск. Копия ещё ни разу не получала записи с сервера —
// не записывается: пустая копия только помешала бы понять, что данных без сети нет.
// Ошибка записи не мешает работе с сервером: копия обновится в следующий раз.
func (c *cachedClient) save() {
	if c.snap == nil || c.snap.SyncedAt.IsZero() {
		return
	}
	_ = offline.Save(c.path, c.key, c.snap)
}

func toSecret(r offline.Record) *domain.Secret {
	return &domain.Secret{BlindIndex: r.BlindIndex, Data: r.Data, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func noCache(ctx context.Context) bool {
	v, _ := ctx.Value(noCacheKey{}).(bool)
	return v
}

// isNetworkError — сервер недоступен (сеть, таймаут), но не ошибка сертификата.
func isNetworkError(err error) bool {
	return errors.Is(err, domain.ErrUnavailable) && ErrorCode(err) == CodeUnavailable
}
