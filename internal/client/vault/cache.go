package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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
// когда сервер недоступен. Изменения без сервера ставятся в очередь (offline.PendingOp)
// и отправляются, когда сервер снова доступен (sync); если запись за это время изменили
// на другом устройстве, изменение ждёт решения пользователя (конфликт).
//
// Все методы вызываются под Vault.mu, поэтому своей блокировки у cachedClient нет.
type cachedClient struct {
	remote grpcclient.SecretsClient
	// path — файл копии; пустой — копия выключена, клиент только передаёт запросы.
	path string
	now  func() time.Time
	// retryAfter — сколько после сбоя сети читать из копии, не обращаясь к серверу.
	retryAfter time.Duration
	// queue — изменения без сервера ставятся в очередь; иначе возвращается ошибка сети.
	queue bool

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

func newCachedClient(remote grpcclient.SecretsClient, path string, retryAfter time.Duration, queue bool) *cachedClient {
	return &cachedClient{remote: remote, path: path, now: time.Now, retryAfter: retryAfter, queue: queue}
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
	if noCache(ctx) {
		return c.remote.ListSecrets(ctx)
	}
	if c.skipRemote(ctx) {
		return c.view(), nil
	}
	rctx, cancel := c.readContext(ctx)
	secrets, err := c.remote.ListSecrets(rctx)
	cancel()
	if err != nil {
		if c.fallback(ctx, err) {
			return c.view(), nil
		}
		return nil, err
	}
	c.online()
	if c.snap == nil {
		return secrets, nil
	}
	c.snap.Records = make([]offline.Record, 0, len(secrets))
	for _, s := range secrets {
		c.snap.Records = append(c.snap.Records, offline.Record{
			BlindIndex: s.BlindIndex, Data: s.Data, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		})
	}
	c.snap.SyncedAt = c.now()
	c.save()
	return c.view(), nil
}

func (c *cachedClient) GetSecret(ctx context.Context, blindIndex string) (*domain.Secret, error) {
	if noCache(ctx) {
		return c.remote.GetSecret(ctx, blindIndex)
	}
	// Неотправленное изменение важнее версии на сервере: показываем его.
	if c.skipRemote(ctx) || c.findOp(blindIndex) >= 0 {
		return c.viewSecret(blindIndex)
	}
	rctx, cancel := c.readContext(ctx)
	secret, err := c.remote.GetSecret(rctx, blindIndex)
	cancel()
	if err != nil {
		if c.fallback(ctx, err) {
			return c.viewSecret(blindIndex)
		}
		return nil, err
	}
	c.online()
	// Копия на диске обновится при следующем списке; в памяти — сразу.
	c.setRecord(offline.Record{
		BlindIndex: secret.BlindIndex, Data: secret.Data, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt,
	})
	return secret, nil
}

func (c *cachedClient) CreateSecret(ctx context.Context, blindIndex string, data []byte) error {
	return c.write(ctx, offline.OpCreate, blindIndex, data, func(ctx context.Context) error {
		return c.remote.CreateSecret(ctx, blindIndex, data)
	})
}

func (c *cachedClient) UpdateSecret(ctx context.Context, blindIndex string, data []byte) error {
	return c.write(ctx, offline.OpUpdate, blindIndex, data, func(ctx context.Context) error {
		return c.remote.UpdateSecret(ctx, blindIndex, data)
	})
}

func (c *cachedClient) DeleteSecret(ctx context.Context, blindIndex string) error {
	return c.write(ctx, offline.OpDelete, blindIndex, nil, func(ctx context.Context) error {
		return c.remote.DeleteSecret(ctx, blindIndex)
	})
}

// write выполняет изменение на сервере, а без сервера ставит его в очередь (enqueue).
// В очередь идёт и изменение записи, у которой уже есть неотправленное изменение:
// иначе порядок изменений на сервере перепутался бы.
func (c *cachedClient) write(
	ctx context.Context, kind offline.OpKind, blindIndex string, data []byte, remote func(context.Context) error,
) error {
	if c.snap == nil || noCache(ctx) {
		return remote(ctx)
	}
	if c.queue && (c.skipRemote(ctx) || c.findOp(blindIndex) >= 0) {
		return c.enqueue(kind, blindIndex, data)
	}
	err := remote(ctx)
	if isNetworkError(err) {
		c.markOffline()
		if c.queue {
			return c.enqueue(kind, blindIndex, data)
		}
	}
	if err != nil {
		return err
	}
	c.online()
	now := c.now()
	switch kind {
	case offline.OpDelete:
		c.removeRecord(blindIndex)
	case offline.OpUpdate:
		createdAt := now
		if i := c.findRecord(blindIndex); i >= 0 {
			createdAt = c.snap.Records[i].CreatedAt
		}
		c.setRecord(offline.Record{BlindIndex: blindIndex, Data: data, CreatedAt: createdAt, UpdatedAt: now})
	default:
		c.setRecord(offline.Record{BlindIndex: blindIndex, Data: data, CreatedAt: now, UpdatedAt: now})
	}
	c.save()
	return nil
}

// enqueue ставит изменение в очередь, объединяя его с прежним изменением той же записи.
// Проверяет его так же, как сервер: создать можно только новую запись, изменить и
// удалить — существующую.
func (c *cachedClient) enqueue(kind offline.OpKind, blindIndex string, data []byte) error {
	_, exists := c.viewRecord(blindIndex)
	switch {
	case kind == offline.OpCreate && exists:
		return domain.ErrAlreadyExists
	case kind != offline.OpCreate && !exists:
		return domain.ErrNotFound
	}
	now := c.now()

	i := c.findOp(blindIndex)
	if i < 0 {
		op := offline.PendingOp{Kind: kind, BlindIndex: blindIndex, Data: data, EditedAt: now}
		if kind != offline.OpCreate {
			r := c.snap.Records[c.findRecord(blindIndex)]
			op.BaseHash = hashData(r.Data)
			if kind == offline.OpDelete {
				op.Data = r.Data
			}
		}
		c.snap.Pending = append(c.snap.Pending, op)
		c.save()
		return nil
	}

	op := &c.snap.Pending[i]
	// Новое изменение меняет суть конфликта; при следующей отправке он определится заново.
	op.Conflict = nil
	op.EditedAt = now
	switch {
	case op.Kind == offline.OpCreate && kind == offline.OpDelete:
		// Запись, созданная без сервера, удалена без сервера: отправлять нечего.
		c.snap.Pending = slices.Delete(c.snap.Pending, i, i+1)
	case kind == offline.OpDelete:
		op.Kind = offline.OpDelete
	case op.Kind == offline.OpDelete:
		// Удалённую запись создали заново: для сервера это изменение прежней.
		op.Kind, op.Data = offline.OpUpdate, data
	default:
		// create + update остаётся create, update + update — update с новыми данными.
		op.Data = data
	}
	c.save()
	return nil
}

// sync отправляет очередь на сервер. Изменения с конфликтом пропускаются, пока
// пользователь его не разрешит; новые конфликты запоминаются. Сбой сети прерывает
// отправку (остаток уйдёт в следующий раз), как и отказ сервера принять токен.
func (c *cachedClient) sync(ctx context.Context) error {
	if c.snap == nil || len(c.snap.Pending) == 0 {
		return nil
	}
	defer c.save()
	for _, op := range slices.Clone(c.snap.Pending) {
		if op.Conflict != nil {
			continue
		}
		conflict, err := c.syncOp(ctx, op)
		switch {
		case isNetworkError(err):
			c.markOffline()
			return err
		case errors.Is(err, domain.ErrInvalidCredentials):
			return err
		case err != nil:
			conflict = &offline.Conflict{Kind: offline.ConflictRejected, Message: err.Error()}
		}
		c.online()
		i := c.findOp(op.BlindIndex)
		if conflict != nil {
			c.snap.Pending[i].Conflict = conflict
			continue
		}
		c.snap.Pending = slices.Delete(c.snap.Pending, i, i+1)
		c.applySynced(op)
	}
	return nil
}

// syncOp отправляет одно изменение. Прежде чем изменить или удалить запись, сверяет её
// версию на сервере с той, от которой начато изменение.
func (c *cachedClient) syncOp(ctx context.Context, op offline.PendingOp) (*offline.Conflict, error) {
	if op.Kind == offline.OpCreate {
		err := c.remote.CreateSecret(ctx, op.BlindIndex, op.Data)
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return nil, err
		}
		server, err := c.remote.GetSecret(ctx, op.BlindIndex)
		if err != nil {
			return nil, err
		}
		return serverConflict(offline.ConflictExists, server), nil
	}

	server, err := c.remote.GetSecret(ctx, op.BlindIndex)
	switch {
	case errors.Is(err, domain.ErrNotFound) && op.Kind == offline.OpDelete:
		return nil, nil // уже удалена
	case errors.Is(err, domain.ErrNotFound):
		return &offline.Conflict{Kind: offline.ConflictDeletedOnServer}, nil
	case err != nil:
		return nil, err
	case !bytes.Equal(hashData(server.Data), op.BaseHash):
		if op.Kind == offline.OpDelete {
			return serverConflict(offline.ConflictChangedOnServer, server), nil
		}
		return serverConflict(offline.ConflictChanged, server), nil
	}

	if op.Kind == offline.OpDelete {
		err = c.remote.DeleteSecret(ctx, op.BlindIndex)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	err = c.remote.UpdateSecret(ctx, op.BlindIndex, op.Data)
	if errors.Is(err, domain.ErrNotFound) {
		return &offline.Conflict{Kind: offline.ConflictDeletedOnServer}, nil
	}
	return nil, err
}

func serverConflict(kind offline.ConflictKind, server *domain.Secret) *offline.Conflict {
	return &offline.Conflict{Kind: kind, ServerData: server.Data, ServerUpdatedAt: server.UpdatedAt}
}

// applySynced переносит отправленное изменение в записи сервера.
func (c *cachedClient) applySynced(op offline.PendingOp) {
	if op.Kind == offline.OpDelete {
		c.removeRecord(op.BlindIndex)
		return
	}
	now := c.now()
	createdAt := now
	if i := c.findRecord(op.BlindIndex); i >= 0 {
		createdAt = c.snap.Records[i].CreatedAt
	}
	c.setRecord(offline.Record{BlindIndex: op.BlindIndex, Data: op.Data, CreatedAt: createdAt, UpdatedAt: now})
}

// Варианты разрешения конфликта.
type Resolution string

const (
	// ResolveMine — отправить версию с устройства поверх версии на сервере.
	ResolveMine Resolution = "mine"
	// ResolveServer — отказаться от изменения с устройства.
	ResolveServer Resolution = "server"
	// ResolveBoth — оставить версию на сервере, а версию с устройства сохранить копией.
	ResolveBoth Resolution = "both"
)

// resolve применяет решение по конфликту записи blindIndex (кроме ResolveBoth, которое
// собирает Vault). После ResolveMine изменение снова в очереди и уйдёт при sync.
func (c *cachedClient) resolve(blindIndex string, choice Resolution) error {
	i := c.findOp(blindIndex)
	if i < 0 || c.snap.Pending[i].Conflict == nil {
		return ErrNoConflict
	}
	op := &c.snap.Pending[i]
	conflict := op.Conflict
	// Запись сервера в копии — та, с которой сравнивали.
	if conflict.ServerData != nil {
		c.setRecord(offline.Record{
			BlindIndex: blindIndex, Data: conflict.ServerData,
			CreatedAt: conflict.ServerUpdatedAt, UpdatedAt: conflict.ServerUpdatedAt,
		})
	} else if conflict.Kind != offline.ConflictRejected {
		c.removeRecord(blindIndex)
	}

	switch choice {
	case ResolveServer:
		c.snap.Pending = slices.Delete(c.snap.Pending, i, i+1)
	case ResolveMine:
		switch conflict.Kind {
		case offline.ConflictDeletedOnServer:
			op.Kind, op.BaseHash = offline.OpCreate, nil
		case offline.ConflictExists:
			op.Kind, op.BaseHash = offline.OpUpdate, hashData(conflict.ServerData)
		case offline.ConflictChanged, offline.ConflictChangedOnServer:
			op.BaseHash = hashData(conflict.ServerData)
		}
		op.Conflict = nil
	default:
		return fmt.Errorf("vault: resolution %q is not supported here", choice)
	}
	c.save()
	return nil
}

// pendingOps возвращает копию очереди.
func (c *cachedClient) pendingOps() []offline.PendingOp {
	if c.snap == nil {
		return nil
	}
	return slices.Clone(c.snap.Pending)
}

// view — записи с учётом неотправленных изменений.
func (c *cachedClient) view() []*domain.Secret {
	result := make([]*domain.Secret, 0, len(c.snap.Records)+len(c.snap.Pending))
	for _, r := range c.snap.Records {
		if v, ok := c.viewRecord(r.BlindIndex); ok {
			result = append(result, toSecret(v))
		}
	}
	for _, op := range c.snap.Pending {
		if op.Kind != offline.OpDelete && c.findRecord(op.BlindIndex) < 0 {
			v, _ := c.viewRecord(op.BlindIndex)
			result = append(result, toSecret(v))
		}
	}
	return result
}

// viewRecord — запись с учётом неотправленного изменения; false — записи нет.
func (c *cachedClient) viewRecord(blindIndex string) (offline.Record, bool) {
	var r offline.Record
	found := false
	if i := c.findRecord(blindIndex); i >= 0 {
		r, found = c.snap.Records[i], true
	}
	i := c.findOp(blindIndex)
	if i < 0 {
		return r, found
	}
	op := c.snap.Pending[i]
	if op.Kind == offline.OpDelete {
		return offline.Record{}, false
	}
	if !found {
		r = offline.Record{BlindIndex: blindIndex, CreatedAt: op.EditedAt}
	}
	r.Data, r.UpdatedAt = op.Data, op.EditedAt
	return r, true
}

func (c *cachedClient) viewSecret(blindIndex string) (*domain.Secret, error) {
	if c.snap == nil {
		return nil, domain.ErrNotFound
	}
	if r, ok := c.viewRecord(blindIndex); ok {
		return toSecret(r), nil
	}
	return nil, domain.ErrNotFound
}

func (c *cachedClient) findRecord(blindIndex string) int {
	if c.snap == nil {
		return -1
	}
	return slices.IndexFunc(c.snap.Records, func(r offline.Record) bool { return r.BlindIndex == blindIndex })
}

func (c *cachedClient) findOp(blindIndex string) int {
	if c.snap == nil {
		return -1
	}
	return slices.IndexFunc(c.snap.Pending, func(op offline.PendingOp) bool { return op.BlindIndex == blindIndex })
}

// setRecord добавляет или заменяет запись сервера в копии в памяти.
func (c *cachedClient) setRecord(r offline.Record) {
	if c.snap == nil {
		return
	}
	if i := c.findRecord(r.BlindIndex); i >= 0 {
		c.snap.Records[i] = r
		return
	}
	c.snap.Records = append(c.snap.Records, r)
}

func (c *cachedClient) removeRecord(blindIndex string) {
	if i := c.findRecord(blindIndex); i >= 0 {
		c.snap.Records = slices.Delete(c.snap.Records, i, i+1)
	}
}

// save записывает копию на диск. Копия, которая ещё ни разу не получала записи с
// сервера и не хранит изменений, не записывается: пустая копия только помешала бы
// понять, что данных без сети нет. Ошибка записи не мешает работе с сервером.
func (c *cachedClient) save() {
	if c.snap == nil || (c.snap.SyncedAt.IsZero() && len(c.snap.Pending) == 0) {
		return
	}
	_ = offline.Save(c.path, c.key, c.snap)
}

func hashData(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
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
