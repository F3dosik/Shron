package mobile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/F3dosik/Shron/internal/client/export"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
)

// MaxExportFileSize — наибольший размер файла экспорта для импорта (как на десктопе).
const MaxExportFileSize = 1 << 30

// ExportFileName — имя файла экспорта по умолчанию: shron-<логин>-<дата>.gkx (как на
// десктопе); у хранилища без сервера — shron-<дата>.gkx.
func (a *App) ExportFileName() string {
	date := time.Now().Format("2006-01-02")
	if v, err := a.currentVault(); err == nil && v.CurrentLogin() != "" {
		return fmt.Sprintf("shron-%s-%s.gkx", v.CurrentLogin(), date)
	}
	return fmt.Sprintf("shron-%s.gkx", date)
}

// ExportVault возвращает содержимое файла экспорта (.gkx) со всеми записями, включая
// записи только на этом устройстве, зашифрованное паролем экспорта password. Файл
// сохраняет приложение (на Android — через Storage Access Framework). Его можно
// импортировать в любой аккаунт на любом сервере, на десктопе и в CLI.
func (a *App) ExportVault(password string) ([]byte, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	if len(password) < export.MinPasswordLength {
		return nil, validationError(export.ErrWeakPassword.Error())
	}
	secrets, err := v.ExportSecrets(context.Background())
	if err != nil {
		return nil, toError(err)
	}
	data, err := export.Encrypt(&export.Archive{
		ExportedAt: time.Now().UTC(),
		Login:      v.CurrentLogin(),
		Secrets:    secrets,
	}, password)
	if err != nil {
		return nil, toError(err)
	}
	return data, nil
}

// ImportResult — итог импорта.
type ImportResult struct {
	// SourceLogin — учётка, из которой сделан экспорт; ExportedAt — когда, мс Unix.
	SourceLogin string
	ExportedAt  int64
	Total       int
	Created     int
	Updated     int
	Skipped     int
	// Failed — число записей с ошибкой; FailedDetails — «название: ошибка» по одной на
	// строку (gomobile не передаёт срезы структур).
	Failed        int
	FailedDetails string
	// Interrupted — импорт прерван (нет связи, сеанс истёк, лимит записей); остальные
	// записи не обработаны. Причина — в InterruptReason.
	Interrupted     bool
	InterruptReason string
}

// ImportVault импортирует записи из файла экспорта data (.gkx) с паролем экспорта password.
// Записи с тем же названием и типом пропускаются или, при overwrite, перезаписываются.
// Неверный пароль — код WRONG_PASSWORD, не файл экспорта — VALIDATION.
func (a *App) ImportVault(data []byte, password string, overwrite bool) (*ImportResult, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	if len(data) > MaxExportFileSize {
		return nil, validationError("Файл слишком большой для экспорта Shron")
	}
	archive, err := export.Decrypt(data, password)
	if err != nil {
		code := CodeWrongPassword
		message := "Неверный пароль экспорта"
		if !errors.Is(err, export.ErrWrongPassword) {
			code, message = CodeValidation, "Это не файл экспорта Shron или он повреждён"
		}
		return nil, &Error{Code: code, Message: message}
	}

	report, err := export.Import(context.Background(), v, archive, overwrite, isFatalImportError)
	result := &ImportResult{
		SourceLogin: archive.Login,
		ExportedAt:  unixMilli(archive.ExportedAt),
		Total:       len(archive.Secrets),
		Created:     report.Created,
		Updated:     report.Updated,
		Skipped:     report.Skipped,
		Failed:      len(report.Failed),
	}
	details := make([]string, 0, len(report.Failed))
	for _, f := range report.Failed {
		details = append(details, f.Name+": "+f.Error)
	}
	result.FailedDetails = strings.Join(details, "\n")
	if err != nil {
		result.Interrupted = true
		var appErr *Error
		if errors.As(toError(err), &appErr) {
			result.InterruptReason = appErr.Message
		}
	}
	return result, nil
}

// isFatalImportError — ошибки, после которых продолжать импорт бессмысленно.
func isFatalImportError(err error) bool {
	return errors.Is(err, vault.ErrLocked) ||
		errors.Is(err, vault.ErrSessionExpired) ||
		errors.Is(err, domain.ErrUnavailable) ||
		errors.Is(err, domain.ErrResourceExhausted)
}
