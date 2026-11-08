package mobile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/domain"
)

// importedLabel — приписка к названию записи, перенесённой из хранилища без сервера,
// если такое название в аккаунте уже занято.
const importedLabel = "с телефона"

// UseStandalone переключает приложение на хранилище без сервера (State.Standalone) и
// сохраняет таймаут автоблокировки. Хранилище создаётся отдельно (CreateStandalone), если
// его ещё нет. Из аккаунта на сервере сначала нужно выйти: иначе его копия на устройстве
// и записи только на устройстве пропали бы молча.
func (a *App) UseStandalone(autoLockMinutes int) error {
	if autoLockMinutes < 0 || autoLockMinutes > 24*60 {
		return validationError("Автоблокировка — от 0 до 1440 минут")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.settings.Standalone && a.vault != nil && a.vault.CurrentLogin() != "" {
		return validationError("Чтобы пользоваться Shron без сервера, сначала выйдите из аккаунта")
	}
	ca, err := loadCACert(a.dir)
	if err != nil {
		return toError(err)
	}
	s := settings{Standalone: true, AutoLockMinutes: autoLockMinutes}
	if err := saveSettings(a.dir, s, ca); err != nil {
		return toError(err)
	}
	if a.settings.ServerAddress != "" {
		_ = os.Remove(filepath.Join(a.dir, sessionFile))
		_ = offline.Remove(filepath.Join(a.dir, offline.FileName))
		_ = offline.Remove(filepath.Join(a.dir, offline.FileName+offline.StaleSuffix))
	}
	a.settings = s
	return a.openVaultLocked()
}

// CreateStandalone создаёт хранилище без сервера с мастер-паролем password и
// разблокирует его.
func (a *App) CreateStandalone(password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	if len(password) < minPasswordLength {
		return validationError(fmt.Sprintf("Пароль должен быть не короче %d символов", minPasswordLength))
	}
	return toError(v.CreateStandalone(password, domain.DefaultKDFParams))
}

// ImportStandalone переносит записи хранилища без сервера в аккаунт, в который выполнен
// вход (State.StandaloneRecords). password — мастер-пароль хранилища без сервера.
// Возвращает число перенесённых записей; неверный пароль — код WRONG_PASSWORD.
func (a *App) ImportStandalone(password string) (int, error) {
	v, err := a.currentVault()
	if err != nil {
		return 0, err
	}
	if password == "" {
		return 0, validationError("Введите пароль хранилища")
	}
	n, err := v.ImportStandalone(context.Background(), a.standalonePath(), password, importedLabel)
	return n, toError(err)
}

// DiscardStandalone удаляет хранилище без сервера, оставшееся после подключения к серверу.
func (a *App) DiscardStandalone() error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.DiscardStandalone(a.standalonePath()))
}
