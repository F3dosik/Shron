package mobile

import (
	"context"

	"github.com/F3dosik/Shron/internal/client/vault"
)

// StatusLocal — запись только на этом устройстве (SecretSummary.Status).
const StatusLocal = string(vault.StatusLocal)

// recoveredLabel — приписка к названию восстановленной записи, если название занято.
const recoveredLabel = "восстановлено"

// SetLocal переносит запись на это устройство (local == true: она удаляется с сервера и
// хранится только здесь) или обратно на сервер (local == false).
func (a *App) SetLocal(name, secretType string, local bool) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	t, err := parseType(secretType)
	if err != nil {
		return err
	}
	return toError(v.SetLocal(context.Background(), name, t, local))
}

// RecoverLocalRecords возвращает записи только на этом устройстве, зашифрованные
// прежним паролем (State.StaleLocalRecords), — после смены пароля на другом устройстве.
// Возвращает число восстановленных записей; неверный пароль — код WRONG_PASSWORD.
func (a *App) RecoverLocalRecords(oldPassword string) (int, error) {
	v, err := a.currentVault()
	if err != nil {
		return 0, err
	}
	if oldPassword == "" {
		return 0, validationError("Введите прежний пароль")
	}
	n, err := v.RecoverLocalRecords(context.Background(), oldPassword, recoveredLabel)
	return n, toError(err)
}

// DiscardStaleLocalRecords удаляет записи только на этом устройстве, зашифрованные
// прежним паролем, если прежний пароль не вспомнить.
func (a *App) DiscardStaleLocalRecords() error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.DiscardStaleLocalRecords())
}
