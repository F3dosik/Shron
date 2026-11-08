package mobile

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
)

// Коды ошибок (поле code в тексте исключения). Константы видны в Kotlin как Mobile.Code*.
const (
	CodeWrongPassword          = string(vault.CodeWrongPassword)
	CodeLocked                 = string(vault.CodeLocked)
	CodeNotSignedIn            = string(vault.CodeNotSignedIn)
	CodeSessionExpired         = string(vault.CodeSessionExpired)
	CodeKeyRejected            = string(vault.CodeKeyRejected)
	CodePasswordChangeRequired = string(vault.CodePasswordChangeRequired)
	CodeIntegrity              = string(vault.CodeIntegrity)
	CodeKDFDowngrade           = string(vault.CodeKDFDowngrade)
	CodeLimitExceeded          = string(vault.CodeLimitExceeded)
	CodeSecretsChanged         = string(vault.CodeSecretsChanged)
	CodePermissionDenied       = string(vault.CodePermissionDenied)
	CodeNotFound               = string(vault.CodeNotFound)
	CodeAlreadyExists          = string(vault.CodeAlreadyExists)
	CodeInvalidArgument        = string(vault.CodeInvalidArgument)
	CodeUnavailable            = string(vault.CodeUnavailable)
	CodeBadCertificate         = string(vault.CodeBadCertificate)
	CodeNotSupported           = string(vault.CodeNotSupported)
	CodeInternal               = string(vault.CodeInternal)
	CodeNoConflict             = string(vault.CodeNoConflict)
	CodeUnsyncedChanges        = string(vault.CodeUnsyncedChanges)

	// CodeNotConfigured — не задан адрес сервера.
	CodeNotConfigured = "NOT_CONFIGURED"
	// CodeValidation — данные формы не прошли проверку на клиенте.
	CodeValidation = "VALIDATION"
)

// Error — ошибка для приложения. gomobile передаёт в Kotlin только текст ошибки
// (Exception.message), поэтому Error() — JSON {"code": ..., "message": ...}.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// validationError — ошибка проверки формы.
func validationError(message string) error {
	return &Error{Code: CodeValidation, Message: message}
}

// messages — понятные пользователю сообщения по кодам. Для кодов, где важны
// подробности с сервера (лимиты, неверные данные), используется текст ошибки.
var messages = map[string]string{
	CodeWrongPassword:          "Неверный мастер-пароль",
	CodeLocked:                 "Хранилище заблокировано",
	CodeNotSignedIn:            "Вход не выполнен",
	CodeSessionExpired:         "Сеанс истёк или завершён — введите мастер-пароль",
	CodeKeyRejected:            "Мастер-пароль изменился — введите новый пароль",
	CodePasswordChangeRequired: "Пароль временный — задайте свой",
	CodeIntegrity:              "Нарушена целостность: сервер вернул не ту запись",
	CodeSecretsChanged:         "Записи изменились во время смены пароля — повторите",
	CodeNotFound:               "Не найдено",
	CodeAlreadyExists:          "Уже существует",
	CodeUnavailable:            "Сервер недоступен",
	CodeNoConflict:             "Конфликт уже разрешён",
	CodeUnsyncedChanges:        "Есть изменения, не отправленные на сервер: дождитесь связи и разрешите конфликты",
	CodeBadCertificate:         "Сертификат сервера не прошёл проверку — проверьте ca.crt и адрес",
}

// toError переводит ошибку в *Error с кодом и сообщением.
func toError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}

	code := string(vault.ErrorCode(err))
	message, ok := messages[code]
	var downgrade *domain.KDFDowngradeError
	switch {
	case errors.As(err, &downgrade):
		message = fmt.Sprintf("Сервер прислал более слабые параметры Argon2id, чем раньше (%s вместо %s). "+
			"Так подменённый сервер мог бы облегчить подбор мастер-пароля; пароль не отправлен. "+
			"Принимайте новые параметры, только если сами сменили пароль с ними на другом устройстве.",
			downgrade.Received, downgrade.Known)
	case errors.Is(err, vault.ErrStandalone):
		message = "Без сервера это недоступно"
	case errors.Is(err, vault.ErrLocalUnavailable):
		message = "Записи только на устройстве недоступны: нет копии хранилища"
	case errors.Is(err, domain.ErrInvalidCardNumber):
		code, message = CodeValidation, "Неверный номер карты"
	case errors.Is(err, domain.ErrInvalidCardExpiry):
		code, message = CodeValidation, "Срок действия карты — в формате ММ/ГГ"
	case errors.Is(err, domain.ErrInvalidCardCVV):
		code, message = CodeValidation, "CVV — 3 или 4 цифры"
	case errors.Is(err, domain.ErrInvalidCardHolder):
		code, message = CodeValidation, "Укажите держателя карты"
	case errors.Is(err, domain.ErrGroupTooLong):
		code, message = CodeValidation, fmt.Sprintf("Название группы — не длиннее %d символов", domain.MaxGroupLength)
	case !ok:
		message = err.Error()
	}
	return &Error{Code: code, Message: message}
}
