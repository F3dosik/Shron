package vault

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/pkg/crypto"
)

// Хранилище без сервера (Config.Standalone) — файл offline.StandaloneFileName на
// устройстве в формате копии (offline.Snapshot с Standalone): соль и параметры Argon2id
// создаются на устройстве, все записи — записи устройства (Local), зашифрованные теми же
// ключами, что и на сервере. Подпись файла проверяет пароль.
//
// Позже хранилище можно перенести в аккаунт на сервере: после входа на сервер
// ImportStandalone расшифровывает записи паролем хранилища и сохраняет их в аккаунте.

// ErrStandalone — операции нужен сервер, а хранилище работает без него.
var ErrStandalone = fmt.Errorf("%w: not available without a server", domain.ErrNotSupported)

// ErrNoStandaloneVault — хранилища без сервера ещё нет: его нужно создать (CreateStandalone).
var ErrNoStandaloneVault = fmt.Errorf("%w: no vault on this device", ErrNotSignedIn)

// standalone сообщает, что хранилище работает без сервера.
func (v *Vault) standalone() bool {
	return v.cfg.Standalone
}

// HasStandaloneVault сообщает, что хранилище без сервера уже создано.
func (v *Vault) HasStandaloneVault() bool {
	if !v.standalone() {
		return false
	}
	_, err := v.loadStandalone(v.cfg.CachePath)
	return err == nil
}

// CreateStandalone создаёт хранилище без сервера с паролем password и разблокирует его.
func (v *Vault) CreateStandalone(password string, kdf domain.KDFParams) error {
	if !v.standalone() {
		return fmt.Errorf("%w: the vault uses a server", domain.ErrNotSupported)
	}
	if err := kdf.Validate(); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, err := os.Stat(v.cfg.CachePath); err == nil {
		return domain.ErrAlreadyExists
	}
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	masterKey := crypto.DeriveKey(password, salt, kdf)
	snap := &offline.Snapshot{Standalone: true, Salt: salt, KDF: kdf}
	if err := offline.Save(v.cfg.CachePath, masterKey, snap); err != nil {
		clear(masterKey)
		return err
	}
	v.lockLocked()
	return v.unlockStandaloneLocked(snap, masterKey)
}

// loadStandalone читает хранилище без сервера из path (без проверки подписи).
func (v *Vault) loadStandalone(path string) (*offline.Sealed, error) {
	sealed, err := offline.Load(path)
	if err != nil {
		return nil, ErrNoStandaloneVault
	}
	snap := sealed.Unverified()
	if !snap.Standalone || len(snap.Salt) == 0 || snap.KDF.Validate() != nil {
		return nil, ErrNoStandaloneVault
	}
	return sealed, nil
}

// openStandalone проверяет пароль по подписи хранилища path и возвращает его
// содержимое и мастер-ключ.
func (v *Vault) openStandalone(path, password string) (*offline.Snapshot, []byte, error) {
	sealed, err := v.loadStandalone(path)
	if err != nil {
		return nil, nil, err
	}
	stored := sealed.Unverified()
	masterKey := crypto.DeriveKey(password, stored.Salt, stored.KDF)
	snap, err := sealed.Open(masterKey)
	if err != nil {
		clear(masterKey)
		return nil, nil, ErrWrongPassword
	}
	return snap, masterKey, nil
}

// unlockStandaloneLocked разблокирует хранилище без сервера. Ключ переходит во владение
// Vault. Вызывается под v.mu.
func (v *Vault) unlockStandaloneLocked(snap *offline.Snapshot, masterKey []byte) error {
	svc, err := service.NewSecretsService(v.cache, masterKey)
	if err != nil {
		clear(masterKey)
		return fmt.Errorf("vault: %w", err)
	}
	v.masterKey = masterKey
	v.secrets = svc
	v.cache.attach(snap, masterKey, false)
	v.armIdleTimerLocked()
	return nil
}

// unlockStandalone — Unlock для хранилища без сервера.
func (v *Vault) unlockStandalone(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lockLocked()
	snap, masterKey, err := v.openStandalone(v.cfg.CachePath, password)
	if err != nil {
		return err
	}
	return v.unlockStandaloneLocked(snap, masterKey)
}

// unlockStandaloneWithKey — UnlockWithKey для хранилища без сервера. Под v.mu; ключ
// переходит во владение Vault.
func (v *Vault) unlockStandaloneWithKeyLocked(key []byte) error {
	sealed, err := v.loadStandalone(v.cfg.CachePath)
	if err != nil {
		clear(key)
		return err
	}
	snap, err := sealed.Open(key)
	if err != nil {
		clear(key)
		return ErrKeyRejected
	}
	return v.unlockStandaloneLocked(snap, key)
}

// checkStandalonePasswordLocked проверяет пароль разблокированного хранилища без сервера.
// Вызывается под v.mu.
func (v *Vault) checkStandalonePasswordLocked(password string) error {
	if v.secrets == nil || v.cache.snap == nil {
		return ErrLocked
	}
	key := crypto.DeriveKey(password, v.cache.snap.Salt, v.cache.snap.KDF)
	defer clear(key)
	if subtle.ConstantTimeCompare(key, v.masterKey) != 1 {
		return ErrWrongPassword
	}
	return nil
}

// changeStandalonePassword — ChangePassword для хранилища без сервера: новая соль,
// записи перешифровываются ключами нового пароля.
func (v *Vault) changeStandalonePassword(oldPassword, newPassword string, kdf domain.KDFParams) error {
	if err := kdf.Validate(); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkStandalonePasswordLocked(oldPassword); err != nil {
		return err
	}
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	newKey := crypto.DeriveKey(newPassword, salt, kdf)
	resealed, err := v.resealLocalLocked(v.secrets, newKey)
	if err != nil {
		clear(newKey)
		return err
	}
	defer resealed.wipe()
	snap := &offline.Snapshot{Standalone: true, Salt: salt, KDF: kdf}
	if resealed != nil {
		snap.Local = resealed.records
	}
	if err := offline.Save(v.cfg.CachePath, newKey, snap); err != nil {
		clear(newKey)
		return err
	}
	v.lockLocked()
	return v.unlockStandaloneLocked(snap, newKey)
}

// deleteStandalone — DeleteAccount для хранилища без сервера: после проверки пароля
// файл хранилища удаляется.
func (v *Vault) deleteStandalone(password string) error {
	v.mu.Lock()
	if v.secrets == nil {
		// Удалить можно и заблокированное хранилище: пароль проверит подпись.
		_, key, err := v.openStandalone(v.cfg.CachePath, password)
		if err != nil {
			v.mu.Unlock()
			return err
		}
		clear(key)
	} else if err := v.checkStandalonePasswordLocked(password); err != nil {
		v.mu.Unlock()
		return err
	}
	if err := offline.Remove(v.cfg.CachePath); err != nil {
		v.mu.Unlock()
		return err
	}
	locked := v.lockLocked()
	v.mu.Unlock()
	if locked {
		v.notifyLock(LockSignedOut)
	}
	return nil
}

// StandaloneRecords возвращает число записей в хранилище без сервера path, которое
// можно перенести в текущий аккаунт (ImportStandalone); 0 — его нет.
func (v *Vault) StandaloneRecords(path string) int {
	if v.standalone() {
		return 0
	}
	sealed, err := v.loadStandalone(path)
	if err != nil {
		return 0
	}
	return len(sealed.Unverified().Local)
}

// ImportStandalone переносит записи хранилища без сервера path в аккаунт: расшифровывает
// их паролем этого хранилища и сохраняет на сервере (без сети — в очередь изменений).
// Запись, чьё название уже занято, сохраняется с припиской copyLabel. Перенесённые
// записи удаляются из файла по одной, так что после сбоя перенос можно повторить; пустой
// файл удаляется. Возвращает число перенесённых записей.
func (v *Vault) ImportStandalone(ctx context.Context, path, password, copyLabel string) (int, error) {
	if v.standalone() {
		return 0, ErrStandalone
	}
	var count int
	err := v.withSecrets(ctx, bulkTimeout, func(ctx context.Context, svc service.SecretsService) error {
		snap, key, err := v.openStandalone(path, password)
		if err != nil {
			return err
		}
		defer clear(key)
		source, err := service.NewSecretsService(nil, key)
		if err != nil {
			return fmt.Errorf("vault: %w", err)
		}
		defer source.Wipe()

		for len(snap.Local) > 0 {
			r := snap.Local[0]
			p, err := source.Open(r.BlindIndex, r.Data)
			if err != nil {
				return fmt.Errorf("vault: standalone record: %w", err)
			}
			err = svc.CreateSecret(ctx, p)
			if errors.Is(err, domain.ErrAlreadyExists) {
				p.Name = v.copyName(ctx, svc, p.Name, p.Type, copyLabel, time.Now())
				err = svc.CreateSecret(ctx, p)
			}
			if err != nil {
				return err
			}
			count++
			snap.Local = snap.Local[1:]
			if err := offline.Save(path, key, snap); err != nil {
				return err
			}
		}
		return offline.Remove(path)
	})
	return count, err
}

// DiscardStandalone удаляет хранилище без сервера path (его записи не нужны в аккаунте).
func (v *Vault) DiscardStandalone(path string) error {
	if v.standalone() {
		return ErrStandalone
	}
	return offline.Remove(path)
}

// standaloneAuth — AuthService хранилища без сервера: все операции недоступны.
type standaloneAuth struct{}

func (standaloneAuth) CreateUser(context.Context, string, string, domain.KDFParams) error {
	return ErrStandalone
}

func (standaloneAuth) CreateTemporaryUser(context.Context, string, domain.KDFParams) (string, time.Time, error) {
	return "", time.Time{}, ErrStandalone
}

func (standaloneAuth) Login(context.Context, string, string) (bool, error) {
	return false, ErrStandalone
}

func (standaloneAuth) Unlock(context.Context, string, string) ([]byte, error) {
	return nil, ErrStandalone
}

func (standaloneAuth) Logout(context.Context, bool) error { return ErrStandalone }

func (standaloneAuth) ChangePassword(
	context.Context, string, string, string, domain.KDFParams, service.Reencryptor,
) error {
	return ErrStandalone
}

func (standaloneAuth) DeleteAccount(context.Context, string, string) error { return ErrStandalone }

func (standaloneAuth) RefreshToken(context.Context, string, []byte) error { return ErrStandalone }

func (standaloneAuth) ForgetKDFParams(string) error { return ErrStandalone }
