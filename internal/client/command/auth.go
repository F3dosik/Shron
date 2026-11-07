package command

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/client/session"
	"github.com/F3dosik/Shron/internal/domain"

	"github.com/spf13/cobra"
)

// newAuthCmd создаёт команду для работы с аккаунтом пользователя.
func (c *Commands) newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "управление аккаунтом",
	}
	cmd.AddCommand(
		c.newRegisterCmd(),
		c.newLoginCmd(),
		c.newLogoutCmd(),
		c.newPasswdCmd(),
		c.newDeleteAccountCmd(),
	)
	return cmd
}

// newRegisterCmd создаёт команду регистрации нового пользователя.
func (c *Commands) newRegisterCmd() *cobra.Command {
	var (
		temporary bool
		kdf       kdfFlags
	)
	cmd := &cobra.Command{
		Use:   "register <login>",
		Short: "Регистрация нового пользователя",
		Long: "Регистрация нового пользователя.\n\n" +
			"С флагом --temporary пароль генерируется автоматически, действует ограниченное время\n" +
			"и должен быть сменён при первом входе. Так администратор заводит аккаунт для другого\n" +
			"человека; работает только через административный порт сервера (ADMIN_PORT).\n\n" +
			"Надёжный и запоминаемый мастер-пароль можно получить командой\n" +
			"`shron generate --words 6` — парольная фраза из шести случайных слов.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			login := args[0]
			params, err := kdf.params()
			if err != nil {
				return err
			}

			if temporary {
				password, expiresAt, err := c.authService.CreateTemporaryUser(cmd.Context(), login, params)
				if err != nil {
					return err
				}
				fmt.Printf("Временный пароль для %s: %s\n", login, password)
				fmt.Printf("Действует до %s. При первом входе пользователь задаст свой пароль.\n",
					expiresAt.Local().Format("2006-01-02 15:04"))
				return nil
			}

			password, err := promptNewPassword(promptMasterPassword)
			if err != nil {
				return err
			}

			if err := c.authService.CreateUser(cmd.Context(), login, password, params); err != nil {
				return err
			}

			fmt.Println("Пользователь успешно зарегистрирован. Выполните вход командой 'login'.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&temporary, "temporary", false,
		"создать аккаунт с временным паролем (только через административный порт)")
	kdf.register(cmd)
	return cmd
}

// newLoginCmd создаёт команду входа в систему.
func (c *Commands) newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <login>",
		Short: "Вход в систему",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			login := args[0]

			password, err := promptPassword(promptMasterPassword)
			if err != nil {
				return err
			}

			var changeRequired bool
			err = c.withKDFCheck(login, func() error {
				changeRequired, err = c.authService.Login(cmd.Context(), login, password)
				return err
			})
			if err != nil {
				return err
			}

			if changeRequired {
				fmt.Println("Пароль временный, задайте свой.")
				newPassword, err := promptNewPassword(promptNewMasterPassword)
				if err != nil {
					return err
				}
				if newPassword == password {
					return ErrSamePassword
				}
				// Секретов у учётки с временным паролем быть не может, перешифровывать нечего.
				err = c.authService.ChangePassword(cmd.Context(), login, password, newPassword, domain.DefaultKDFParams, nil)
				if err != nil {
					return err
				}
				fmt.Println("Пароль изменён, вход выполнен.")
				return nil
			}

			fmt.Println("Вход выполнен успешно.")
			return nil
		},
	}
}

// newLogoutCmd создаёт команду выхода из системы.
func (c *Commands) newLogoutCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Выход из системы (отзыв токена на сервере и удаление сеанса)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(c.cfg.SessionPath); os.IsNotExist(err) {
				fmt.Println("Вход не выполнен.")
				return nil
			}

			err := c.authService.Logout(cmd.Context(), all)
			if all && errors.Is(err, domain.ErrInvalidCredentials) {
				return fmt.Errorf("токен этого устройства недействителен (истёк или отозван): " +
					"выполните 'shron auth login' и повторите 'logout --all'")
			}
			if err != nil {
				return err
			}

			if all {
				fmt.Println("Выполнен выход на всех устройствах.")
			} else {
				fmt.Println("Выход выполнен, токен отозван.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "отозвать токены на всех устройствах")
	return cmd
}

// newPasswdCmd создаёт команду смены мастер-пароля.
// Все секреты перешифровываются ключом от нового пароля; на сервер они уходят
// одной транзакцией, поэтому при сбое пароль и данные остаются прежними.
func (c *Commands) newPasswdCmd() *cobra.Command {
	var kdf kdfFlags
	cmd := &cobra.Command{
		Use:   "passwd",
		Short: "Смена мастер-пароля с перешифровкой всех записей",
		Long: "Смена мастер-пароля с перешифровкой всех записей.\n\n" +
			"Новый пароль получает параметры Argon2id из --kdf-time и --kdf-memory, поэтому смена\n" +
			"пароля — также способ изменить стоимость деривации ключа. Новый пароль должен\n" +
			"отличаться от текущего.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := kdf.params()
			if err != nil {
				return err
			}

			sess, err := session.Load(c.cfg.SessionPath)
			if err != nil {
				return fmt.Errorf("не выполнен вход, запустите 'shron auth login': %w", err)
			}

			oldPassword, err := promptPassword(promptCurrentPassword)
			if err != nil {
				return err
			}
			var secretSvc service.SecretsService
			err = c.withKDFCheck(sess.Login, func() error {
				secretSvc, err = c.newSecretService(cmd.Context(), sess.Login, oldPassword)
				return err
			})
			if err != nil {
				return err
			}

			newPassword, err := promptNewPassword(promptNewMasterPassword)
			if err != nil {
				return err
			}
			if newPassword == oldPassword {
				return ErrSamePassword
			}

			reencrypt := func(newMasterKey []byte) ([]domain.ReencryptedSecret, error) {
				return secretSvc.Reencrypt(cmd.Context(), newMasterKey)
			}
			err = c.authService.ChangePassword(cmd.Context(), sess.Login, oldPassword, newPassword, params, reencrypt)
			if errors.Is(err, domain.ErrSecretsChanged) {
				return fmt.Errorf("записи изменились во время смены пароля (например, с другого устройства), " +
					"пароль не изменён; повторите 'shron auth passwd'")
			}
			if err != nil {
				return err
			}

			fmt.Println("Пароль изменён. На остальных устройствах выполните 'shron auth login'.")
			return nil
		},
	}
	kdf.register(cmd)
	return cmd
}

// kdfFlags — флаги параметров Argon2id для команд, задающих новый пароль.
type kdfFlags struct {
	time      uint32
	memoryMiB uint32
}

// register добавляет флаги к команде; по умолчанию — domain.DefaultKDFParams.
func (f *kdfFlags) register(cmd *cobra.Command) {
	cmd.Flags().Uint32Var(&f.time, "kdf-time", domain.DefaultKDFParams.Time,
		"число проходов Argon2id (1–10): больше — медленнее вход и дороже перебор пароля")
	cmd.Flags().Uint32Var(&f.memoryMiB, "kdf-memory", domain.DefaultKDFParams.MemoryKiB/1024,
		"память Argon2id в MiB (19–1024)")
}

// params возвращает проверенные параметры Argon2id.
func (f *kdfFlags) params() (domain.KDFParams, error) {
	params := domain.KDFParams{
		Time:      f.time,
		MemoryKiB: f.memoryMiB * 1024,
		Threads:   domain.DefaultKDFParams.Threads,
	}
	if f.memoryMiB > math.MaxUint32/1024 {
		params.MemoryKiB = math.MaxUint32
	}
	if err := params.Validate(); err != nil {
		return domain.KDFParams{}, err
	}
	return params, nil
}

// newDeleteAccountCmd создаёт команду удаления своей учётки.
func (c *Commands) newDeleteAccountCmd() *cobra.Command {
	var skipConfirm bool
	cmd := &cobra.Command{
		Use:   "delete-account",
		Short: "Безвозвратно удалить свой аккаунт со всеми записями",
		Long: "Безвозвратно удаляет аккаунт текущего пользователя вместе со всеми записями.\n" +
			"Требует мастер-пароль: одного файла сеанса для удаления недостаточно.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := session.Load(c.cfg.SessionPath)
			if err != nil {
				return fmt.Errorf("не выполнен вход, запустите 'shron auth login': %w", err)
			}

			if !skipConfirm {
				fmt.Printf("Аккаунт %s и все его записи будут удалены без возможности восстановления.\n", sess.Login)
				if err := confirmByTyping(sess.Login); err != nil {
					return err
				}
			}

			password, err := promptPassword(promptMasterPassword)
			if err != nil {
				return err
			}

			err = c.withKDFCheck(sess.Login, func() error {
				return c.authService.DeleteAccount(cmd.Context(), sess.Login, password)
			})
			if errors.Is(err, domain.ErrInvalidCredentials) {
				return ErrWrongMasterPassword
			}
			if err != nil {
				return err
			}

			fmt.Println("Аккаунт удалён.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Пропустить подтверждение")
	return cmd
}
