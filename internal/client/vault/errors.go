package vault

import (
	"errors"
	"strings"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/domain"
)

// Ошибки Vault.
var (
	// ErrLocked — операция требует разблокированного хранилища.
	ErrLocked = errors.New("vault is locked")
	// ErrNotSignedIn — нет учётки: нужен вход.
	ErrNotSignedIn = errors.New("not signed in")
	// ErrWrongPassword — неверный пароль.
	ErrWrongPassword = errors.New("wrong password")
	// ErrSessionExpired — сервер не принял токен (истёк или отозван); хранилище
	// заблокировано, нужен пароль.
	ErrSessionExpired = errors.New("session expired")
	// ErrKeyRejected — сервер не принял сохранённый мастер-ключ: пароль сменили
	// (например, на другом устройстве). Ключ больше не годится, нужен пароль.
	ErrKeyRejected = errors.New("master key rejected")
	// ErrNoConflict — у записи нет конфликта, который можно так разрешить.
	ErrNoConflict = errors.New("no such conflict")
	// ErrUnsyncedChanges — операции мешают неотправленные изменения или конфликты.
	ErrUnsyncedChanges = errors.New("unsynced offline changes")
)

// Code — категория ошибки для интерфейса: по ней приложение решает, что показать
// (экран входа, сообщение о сети, предупреждение о целостности и т.п.).
type Code string

// Категории ошибок.
const (
	CodeWrongPassword          Code = "WRONG_PASSWORD"
	CodeLocked                 Code = "LOCKED"
	CodeNotSignedIn            Code = "NOT_SIGNED_IN"
	CodeSessionExpired         Code = "SESSION_EXPIRED"
	CodeKeyRejected            Code = "KEY_REJECTED"
	CodePasswordChangeRequired Code = "PASSWORD_CHANGE_REQUIRED"
	CodeIntegrity              Code = "INTEGRITY"
	CodeKDFDowngrade           Code = "KDF_DOWNGRADE"
	CodeLimitExceeded          Code = "LIMIT_EXCEEDED"
	CodeSecretsChanged         Code = "SECRETS_CHANGED"
	CodePermissionDenied       Code = "PERMISSION_DENIED"
	CodeNotFound               Code = "NOT_FOUND"
	CodeAlreadyExists          Code = "ALREADY_EXISTS"
	CodeInvalidArgument        Code = "INVALID_ARGUMENT"
	CodeUnavailable            Code = "UNAVAILABLE"
	CodeBadCertificate         Code = "BAD_CERTIFICATE"
	CodeNotSupported           Code = "NOT_SUPPORTED"
	CodeInternal               Code = "INTERNAL"
	CodeNoConflict             Code = "NO_CONFLICT"
	CodeUnsyncedChanges        Code = "UNSYNCED_CHANGES"
)

// codes сопоставляет ошибки с категориями; порядок важен для обёрнутых ошибок.
var codes = []struct {
	err  error
	code Code
}{
	{ErrWrongPassword, CodeWrongPassword},
	{ErrLocked, CodeLocked},
	{ErrNotSignedIn, CodeNotSignedIn},
	{ErrSessionExpired, CodeSessionExpired},
	{ErrKeyRejected, CodeKeyRejected},
	{ErrNoConflict, CodeNoConflict},
	{ErrUnsyncedChanges, CodeUnsyncedChanges},
	{domain.ErrPasswordChangeRequired, CodePasswordChangeRequired},
	{service.ErrIntegrity, CodeIntegrity},
	{domain.ErrKDFDowngrade, CodeKDFDowngrade},
	{domain.ErrResourceExhausted, CodeLimitExceeded},
	{domain.ErrSecretsChanged, CodeSecretsChanged},
	{domain.ErrPermissionDenied, CodePermissionDenied},
	{domain.ErrNotFound, CodeNotFound},
	{domain.ErrAlreadyExists, CodeAlreadyExists},
	{domain.ErrInvalidArgument, CodeInvalidArgument},
	{domain.ErrUnavailable, CodeUnavailable},
	{domain.ErrNotSupported, CodeNotSupported},
	{grpcclient.ErrBadCACert, CodeBadCertificate},
	{grpcclient.ErrInsecureWithCert, CodeBadCertificate},
}

// ErrorCode возвращает категорию ошибки; для nil — пустую строку.
func ErrorCode(err error) Code {
	if err == nil {
		return ""
	}
	// Ошибка проверки сертификата приходит от gRPC как Unavailable, но для пользователя
	// это не сбой сети, а неверный или отсутствующий CA-сертификат.
	if errors.Is(err, domain.ErrUnavailable) && strings.Contains(err.Error(), "x509:") {
		return CodeBadCertificate
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return c.code
		}
	}
	return CodeInternal
}
