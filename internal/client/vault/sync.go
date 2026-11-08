package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/domain"
)

// Состояние записи относительно сервера (SecretSummary.Status).
type SyncStatus string

const (
	// StatusSynced — запись такая же, как на сервере.
	StatusSynced SyncStatus = ""
	// StatusPending — изменение сделано без сервера и ещё не отправлено.
	StatusPending SyncStatus = "pending"
	// StatusConflict — изменение не отправлено из-за конфликта.
	StatusConflict SyncStatus = "conflict"
)

// ConflictKind — причина конфликта (см. offline.ConflictKind).
type ConflictKind = offline.ConflictKind

// Conflict — изменение с устройства, которое не удалось отправить на сервер.
type Conflict struct {
	Name string
	Type domain.SecretType
	Kind ConflictKind
	// EditedAt — когда запись изменили на устройстве.
	EditedAt time.Time
	// ServerUpdatedAt — когда запись изменили на сервере; нулевое — на сервере её нет.
	ServerUpdatedAt time.Time
	// Mine — версия с устройства; nil — запись на устройстве удалили.
	Mine *domain.SecretPayload
	// Server — версия на сервере; nil — на сервере записи нет.
	Server *domain.SecretPayload
	// Message — ответ сервера для ConflictRejected.
	Message string
	// Resolutions — допустимые решения.
	Resolutions []Resolution
}

// PendingChanges возвращает число неотправленных изменений и конфликтов среди них.
func (v *Vault) PendingChanges() (pending, conflicts int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, op := range v.cache.pendingOps() {
		pending++
		if op.Conflict != nil {
			conflicts++
		}
	}
	return pending, conflicts
}

// Conflicts возвращает конфликты с расшифрованными версиями записи.
func (v *Vault) Conflicts() ([]Conflict, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secrets == nil {
		return nil, ErrLocked
	}
	var result []Conflict
	for _, op := range v.cache.pendingOps() {
		if op.Conflict == nil {
			continue
		}
		c, err := v.describeConflict(op)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

// describeConflict расшифровывает обе версии записи. Вызывается под v.mu.
func (v *Vault) describeConflict(op offline.PendingOp) (Conflict, error) {
	mine, err := v.secrets.Open(op.BlindIndex, op.Data)
	if err != nil {
		return Conflict{}, fmt.Errorf("vault: conflict: %w", err)
	}
	c := Conflict{
		Name: mine.Name, Type: mine.Type, Kind: op.Conflict.Kind, EditedAt: op.EditedAt,
		ServerUpdatedAt: op.Conflict.ServerUpdatedAt, Message: op.Conflict.Message,
	}
	if op.Kind != offline.OpDelete {
		c.Mine = mine
	}
	if op.Conflict.ServerData != nil {
		if c.Server, err = v.secrets.Open(op.BlindIndex, op.Conflict.ServerData); err != nil {
			return Conflict{}, fmt.Errorf("vault: conflict: %w", err)
		}
	}
	switch c.Kind {
	case offline.ConflictChanged, offline.ConflictExists:
		c.Resolutions = []Resolution{ResolveMine, ResolveServer, ResolveBoth}
	default:
		c.Resolutions = []Resolution{ResolveMine, ResolveServer}
	}
	return c, nil
}

// ResolveConflict разрешает конфликт записи name типа secretType:
//   - ResolveMine — версия с устройства отправляется на сервер (если запись за это время
//     снова изменили, конфликт возникнет заново, уже с новой датой);
//   - ResolveServer — изменение с устройства отбрасывается;
//   - ResolveBoth — на сервере остаётся его версия, а версия с устройства сохраняется
//     новой записью «name (copyLabel, дата изменения)».
func (v *Vault) ResolveConflict(
	ctx context.Context, name string, secretType domain.SecretType, choice Resolution, copyLabel string,
) error {
	return v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		bi := svc.BlindIndex(name, secretType)
		var op *offline.PendingOp
		for _, p := range v.cache.pendingOps() {
			if p.BlindIndex == bi && p.Conflict != nil {
				op = &p
				break
			}
		}
		if op == nil {
			return ErrNoConflict
		}
		c, err := v.describeConflict(*op)
		if err != nil {
			return err
		}
		if !containsResolution(c.Resolutions, choice) {
			return fmt.Errorf("%w: %s", ErrNoConflict, choice)
		}

		if choice == ResolveBoth {
			copyPayload := *c.Mine
			copyPayload.Name = v.copyName(ctx, svc, c.Mine.Name, c.Mine.Type, copyLabel, c.EditedAt)
			if err := v.cache.resolve(bi, ResolveServer); err != nil {
				return err
			}
			copyBI, data, err := svc.Seal(&copyPayload)
			if err != nil {
				return err
			}
			if err := v.cache.enqueue(offline.OpCreate, copyBI, data); err != nil {
				return err
			}
		} else if err := v.cache.resolve(bi, choice); err != nil {
			return err
		}
		v.syncLocked(ctx)
		return nil
	})
}

// copyName подбирает свободное название для копии: «name (label, 08.11 14:20)», при
// совпадении — с номером. Вызывается под v.mu.
func (v *Vault) copyName(
	ctx context.Context, svc service.SecretsService, name string, t domain.SecretType, label string, at time.Time,
) string {
	if strings.TrimSpace(label) == "" {
		label = "копия"
	}
	base := fmt.Sprintf("%s (%s, %s)", name, label, at.Local().Format("02.01 15:04"))
	candidate := base
	for n := 2; ; n++ {
		if _, err := v.cache.viewSecret(svc.BlindIndex(candidate, t)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s %d", base, n)
	}
}

func containsResolution(list []Resolution, r Resolution) bool {
	for _, x := range list {
		if x == r {
			return true
		}
	}
	return false
}

// syncLocked отправляет очередь изменений, если сервер не был недавно недоступен.
// Ошибки сети не мешают операции (она пойдёт по копии); отказ сервера принять токен
// заметит сама операция. Вызывается под v.mu.
func (v *Vault) syncLocked(ctx context.Context) {
	if v.cache.skipRemote(ctx) || noCache(ctx) {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, offlineReadTimeout)
	defer cancel()
	_ = v.cache.sync(rctx)
}

// statusOf — состояние записи относительно сервера. Вызывается под v.mu.
func (v *Vault) statusOf(svc service.SecretsService, name string, t domain.SecretType) SyncStatus {
	if v.standalone() {
		// Без сервера все записи на устройстве: отмечать каждую незачем.
		return StatusSynced
	}
	bi := svc.BlindIndex(name, t)
	if v.cache.findLocal(bi) >= 0 {
		return StatusLocal
	}
	for _, op := range v.cache.pendingOps() {
		if op.BlindIndex == bi {
			if op.Conflict != nil {
				return StatusConflict
			}
			return StatusPending
		}
	}
	return StatusSynced
}
