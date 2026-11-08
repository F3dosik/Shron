package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/pkg/crypto"
)

// Записи «только на этом устройстве» хранятся в копии на устройстве (offline.Snapshot.Local)
// и на сервер не отправляются: сервер не знает ни их содержимого, ни их числа и размера.
// Они зашифрованы теми же ключами, что и записи сервера, поэтому при смене пароля на этом
// устройстве перешифровываются вместе с ними (ChangePassword), а после смены пароля на
// другом устройстве копия откладывается до ввода прежнего пароля (RecoverLocalRecords).
// Потеря устройства или выход из учётки — потеря этих записей: сохранить их можно
// экспортом (ExportSecrets включает их).

// ErrLocalUnavailable — записи только на устройстве недоступны: копия на устройстве
// выключена (Config.CachePath пустой).
var ErrLocalUnavailable = fmt.Errorf("%w: device-only records need an offline copy", domain.ErrNotSupported)

// StatusLocal — запись только на этом устройстве.
const StatusLocal SyncStatus = "local"

// LocalRecords возвращает число записей только на этом устройстве. У заблокированного
// хранилища — по файлу копии без проверки подписи: чтобы предупредить, что выход или
// смена сервера их удалит.
func (v *Vault) LocalRecords() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secrets != nil {
		return v.cache.localCount()
	}
	sealed, err := v.loadCache(v.login)
	if err != nil {
		return 0
	}
	return len(sealed.Unverified().Local)
}

// SetLocal переносит запись name типа secretType на это устройство (local == true): она
// удаляется с сервера и остаётся только здесь; или обратно на сервер (local == false).
// Без сервера перенос ставится в очередь изменений, если она включена.
func (v *Vault) SetLocal(ctx context.Context, name string, secretType domain.SecretType, local bool) error {
	return v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		bi := svc.BlindIndex(name, secretType)
		if local {
			return v.cache.moveToDevice(ctx, bi)
		}
		return v.cache.moveToServer(ctx, bi)
	})
}

// localReseal — записи только на устройстве, перешифрованные новым мастер-ключом.
type localReseal struct {
	key     []byte
	records []offline.Record
}

func (l *localReseal) wipe() {
	if l != nil {
		clear(l.key)
	}
}

// resealLocalLocked перешифровывает записи только на устройстве ключами от newMasterKey.
// nil — таких записей нет. Вызывается под v.mu.
func (v *Vault) resealLocalLocked(svc service.SecretsService, newMasterKey []byte) (*localReseal, error) {
	records := v.cache.localRecords()
	if len(records) == 0 {
		return nil, nil
	}
	target, err := service.NewSecretsService(nil, newMasterKey)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	defer target.Wipe()
	for i, r := range records {
		payload, err := svc.Open(r.BlindIndex, r.Data)
		if err != nil {
			return nil, fmt.Errorf("vault: device-only record: %w", err)
		}
		if records[i].BlindIndex, records[i].Data, err = target.Seal(payload); err != nil {
			return nil, fmt.Errorf("vault: device-only record: %w", err)
		}
	}
	return &localReseal{key: bytes.Clone(newMasterKey), records: records}, nil
}

// saveResealedLocked после смены пароля записывает копию с перешифрованными записями
// только на устройстве, подписанную новым ключом: разблокировка новым паролем её примет.
// Записи сервера в неё не переносятся — они придут с сервера. Вызывается под v.mu.
func (v *Vault) saveResealedLocked(local *localReseal) {
	if local == nil || v.cfg.CachePath == "" {
		return
	}
	snap := &offline.Snapshot{Server: v.cfg.ServerAddress, Login: v.login, Local: local.records}
	_ = offline.Save(v.cfg.CachePath, local.key, snap)
}

// StaleLocalRecords возвращает число записей только на этом устройстве, зашифрованных
// прежним паролем: пароль сменили на другом устройстве, и копия отложена. Вернуть их
// можно прежним паролем (RecoverLocalRecords) или удалить (DiscardStaleLocalRecords).
func (v *Vault) StaleLocalRecords() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	sealed, err := v.loadStale()
	if err != nil {
		return 0
	}
	return len(sealed.Unverified().Local)
}

// loadStale читает отложенную копию текущей учётки. Вызывается под v.mu.
func (v *Vault) loadStale() (*offline.Sealed, error) {
	if v.cfg.CachePath == "" || v.login == "" {
		return nil, errNoCache
	}
	sealed, err := offline.Load(v.cfg.CachePath + offline.StaleSuffix)
	if err != nil {
		return nil, errNoCache
	}
	snap := sealed.Unverified()
	if snap.Server != v.cfg.ServerAddress || !strings.EqualFold(snap.Login, v.login) || len(snap.Local) == 0 {
		return nil, errNoCache
	}
	return sealed, nil
}

// RecoverLocalRecords расшифровывает отложенные записи только на устройстве прежним
// паролем и добавляет их к записям устройства, зашифровав текущим ключом. Запись, чьё
// название уже занято, сохраняется с припиской copyLabel. Возвращает число записей.
func (v *Vault) RecoverLocalRecords(ctx context.Context, oldPassword, copyLabel string) (int, error) {
	var count int
	err := v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		sealed, err := v.loadStale()
		if err != nil {
			return domain.ErrNotFound
		}
		stale := sealed.Unverified()
		if len(stale.Salt) == 0 || stale.KDF.Validate() != nil {
			return ErrWrongPassword
		}
		oldKey := crypto.DeriveKey(oldPassword, stale.Salt, stale.KDF)
		defer clear(oldKey)
		opened, err := sealed.Open(oldKey)
		if err != nil {
			return ErrWrongPassword
		}
		old, err := service.NewSecretsService(nil, oldKey)
		if err != nil {
			return fmt.Errorf("vault: %w", err)
		}
		defer old.Wipe()

		payloads := make([]*domain.SecretPayload, 0, len(opened.Local))
		for _, r := range opened.Local {
			p, err := old.Open(r.BlindIndex, r.Data)
			if err != nil {
				return fmt.Errorf("vault: device-only record: %w", err)
			}
			payloads = append(payloads, p)
		}
		for _, p := range payloads {
			if _, err := v.cache.viewSecret(svc.BlindIndex(p.Name, p.Type)); err == nil {
				p.Name = v.copyName(ctx, svc, p.Name, p.Type, copyLabel, v.cache.now())
			}
			if err := svc.CreateSecret(withLocal(ctx), p); err != nil {
				return err
			}
			count++
		}
		if err := offline.Remove(v.cfg.CachePath + offline.StaleSuffix); err != nil {
			return err
		}
		return nil
	})
	return count, err
}

// DiscardStaleLocalRecords удаляет отложенные записи только на устройстве (прежний
// пароль не вспомнить).
func (v *Vault) DiscardStaleLocalRecords() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cfg.CachePath == "" {
		return nil
	}
	err := offline.Remove(v.cfg.CachePath + offline.StaleSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// SecretStatus возвращает состояние записи name типа secretType относительно сервера
// (StatusLocal — только на этом устройстве). У заблокированного хранилища — StatusSynced.
func (v *Vault) SecretStatus(name string, secretType domain.SecretType) SyncStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secrets == nil {
		return StatusSynced
	}
	return v.statusOf(v.secrets, name, secretType)
}
