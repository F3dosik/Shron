package backend

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
)

// Коды ошибок приложения, дополняющие vault.Code.
const (
	// CodeNotConfigured — не задан адрес сервера.
	CodeNotConfigured vault.Code = "NOT_CONFIGURED"
	// CodeValidation — данные формы не прошли проверку на клиенте.
	CodeValidation vault.Code = "VALIDATION"
)

// Error — ошибка для интерфейса. Wails передаёт в JavaScript только текст ошибки,
// поэтому Error() — это JSON {"code": ..., "message": ...}: интерфейс разбирает код,
// чтобы решить, что делать (например, показать экран разблокировки), и показывает
// сообщение пользователю.
type Error struct {
	Code    vault.Code `json:"code"`
	Message string     `json:"message"`
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
var messages = map[vault.Code]string{
	vault.CodeWrongPassword:          "Неверный мастер-пароль",
	vault.CodeLocked:                 "Хранилище заблокировано",
	vault.CodeNotSignedIn:            "Вход не выполнен",
	vault.CodeSessionExpired:         "Сеанс истёк или завершён — введите мастер-пароль",
	vault.CodePasswordChangeRequired: "Пароль временный — задайте свой",
	vault.CodeIntegrity:              "Нарушена целостность: сервер вернул не ту запись",
	vault.CodeSecretsChanged:         "Записи изменились во время смены пароля — повторите",
	vault.CodeNotFound:               "Не найдено",
	vault.CodeAlreadyExists:          "Уже существует",
	vault.CodeUnavailable:            "Сервер недоступен",
	vault.CodeBadCertificate:         "Сертификат сервера не прошёл проверку — проверьте ca.crt и адрес",
}

// toUIError переводит ошибку в Error с кодом и сообщением.
func toUIError(err error) error {
	if err == nil {
		return nil
	}
	var uiErr *Error
	if errors.As(err, &uiErr) {
		return uiErr
	}

	code := vault.ErrorCode(err)
	message, ok := messages[code]
	var downgrade *domain.KDFDowngradeError
	switch {
	case errors.As(err, &downgrade):
		message = fmt.Sprintf("Сервер прислал более слабые параметры Argon2id, чем раньше (%s вместо %s). "+
			"Так подменённый сервер мог бы облегчить подбор мастер-пароля; пароль не отправлен. "+
			"Принимайте новые параметры, только если сами сменили пароль с ними на другом устройстве.",
			downgrade.Received, downgrade.Known)
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
