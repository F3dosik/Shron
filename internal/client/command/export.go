package command

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/F3dosik/Shron/internal/client/export"
	"github.com/F3dosik/Shron/internal/client/session"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/spf13/cobra"
)

const (
	promptExportPassword        = "Пароль экспорта: "
	promptExportPasswordConfirm = "Повторите пароль экспорта: "
)

// newExportCmd создаёт команду экспорта хранилища в зашифрованный файл.
func (c *Commands) newExportCmd() *cobra.Command {
	var (
		output string
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Экспорт всех записей в зашифрованный файл",
		Long: "Сохраняет все записи в файл, зашифрованный отдельным паролем экспорта\n" +
			"(Argon2id + AES-256-GCM). Файл не зависит от сервера и мастер-пароля: его можно\n" +
			"хранить где угодно и импортировать в любой аккаунт командой `shron import`.\n\n" +
			"Надёжный пароль экспорта: `shron generate --words 6`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				if _, err := os.Stat(output); err == nil {
					return fmt.Errorf("файл %s уже существует (перезаписать: --force)", output)
				}
			}

			sess, err := session.Load(c.cfg.SessionPath)
			if err != nil {
				return fmt.Errorf("не выполнен вход, запустите 'shron auth login': %w", err)
			}
			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}
			infos, err := secretSvc.ListSecrets(cmd.Context())
			if err != nil {
				return err
			}

			password, err := promptExportPasswordTwice()
			if err != nil {
				return err
			}

			archive := &export.Archive{ExportedAt: time.Now().UTC(), Login: sess.Login}
			for _, info := range infos {
				archive.Secrets = append(archive.Secrets, info.SecretPayload)
			}
			data, err := export.Encrypt(archive, password)
			if err != nil {
				return err
			}
			// 0600: файл зашифрован, но посторонним в системе он всё равно ни к чему.
			if err := os.WriteFile(output, data, 0o600); err != nil {
				return fmt.Errorf("запись файла: %w", err)
			}
			fmt.Printf("Экспортировано записей: %d → %s\n", len(archive.Secrets), output)
			fmt.Println("Без пароля экспорта файл не расшифровать — храните пароль отдельно от файла.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "файл экспорта (обязательно), например vault.gkx")
	cmd.Flags().BoolVar(&force, "force", false, "перезаписать существующий файл")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

// newImportCmd создаёт команду импорта секретов из файла экспорта.
func (c *Commands) newImportCmd() *cobra.Command {
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "import <файл>",
		Short: "Импорт записей из файла экспорта",
		Long: "Добавляет в текущий аккаунт записи из файла, созданного `shron export`.\n" +
			"Записи с тем же именем и типом, что уже есть в хранилище, по умолчанию\n" +
			"пропускаются; с --overwrite — перезаписываются.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("чтение файла: %w", err)
			}
			// Сначала пароль экспорта: при ошибке в нём не нужно вводить мастер-пароль.
			password, err := promptPassword(promptExportPassword)
			if err != nil {
				return err
			}
			archive, err := export.Decrypt(data, password)
			if err != nil {
				return err
			}
			fmt.Printf("В файле записей: %d (экспорт %s от %s)\n", len(archive.Secrets), archive.Login,
				archive.ExportedAt.Local().Format("2006-01-02 15:04"))

			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}
			report, err := export.Import(cmd.Context(), secretSvc, archive, overwrite, isFatalImportError)
			printImportReport(report)
			if err != nil {
				return fmt.Errorf("импорт прерван: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "перезаписывать существующие записи")
	return cmd
}

// promptExportPasswordTwice запрашивает пароль экспорта с подтверждением.
func promptExportPasswordTwice() (string, error) {
	password, err := promptPassword(promptExportPassword)
	if err != nil {
		return "", err
	}
	if len(password) < export.MinPasswordLength {
		return "", export.ErrWeakPassword
	}
	confirm, err := promptPassword(promptExportPasswordConfirm)
	if err != nil {
		return "", err
	}
	if password != confirm {
		return "", ErrPasswordsMismatch
	}
	return password, nil
}

// isFatalImportError — ошибки, после которых продолжать импорт бессмысленно.
func isFatalImportError(err error) bool {
	return errors.Is(err, domain.ErrInvalidCredentials) ||
		errors.Is(err, domain.ErrUnavailable) ||
		errors.Is(err, domain.ErrResourceExhausted)
}

// printImportReport выводит итог импорта.
func printImportReport(r export.Report) {
	fmt.Printf("Создано: %d, обновлено: %d, пропущено (уже есть): %d, ошибок: %d\n",
		r.Created, r.Updated, r.Skipped, len(r.Failed))
	for _, f := range r.Failed {
		fmt.Printf("  %s (%s): %s\n", f.Name, f.Type, f.Error)
	}
}
