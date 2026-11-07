package command

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/spf13/cobra"
)

// newAdminCmd создаёт группу административных команд. Они работают только через
// административный порт сервера (SHRON_SERVER=localhost:<ADMIN_PORT>).
func (c *Commands) newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Администрирование сервера (через административный порт)",
		Long: "Административные команды. Работают только при подключении к административному\n" +
			"порту сервера (ADMIN_PORT), который доступен лишь с машины сервера:\n\n" +
			"  SHRON_SERVER=localhost:50052 shron admin delete-user bob",
	}
	cmd.AddCommand(
		c.newListUsersCmd(),
		c.newUserInfoCmd(),
		c.newRevokeSessionsCmd(),
		c.newDeleteUserCmd(),
	)
	return cmd
}

// newDeleteUserCmd создаёт команду удаления пользователя администратором.
func (c *Commands) newDeleteUserCmd() *cobra.Command {
	var skipConfirm bool
	cmd := &cobra.Command{
		Use:   "delete-user <login>",
		Short: "Безвозвратно удалить пользователя со всеми записями",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			login := args[0]

			if !skipConfirm {
				fmt.Printf("Пользователь %s и все его записи будут удалены без возможности восстановления.\n", login)
				if err := confirmByTyping(login); err != nil {
					return err
				}
			}

			if err := c.adminClient.DeleteUser(cmd.Context(), login); err != nil {
				return adminError(err, login)
			}

			fmt.Printf("Пользователь %s удалён.\n", login)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Пропустить подтверждение")
	return cmd
}

// newListUsersCmd создаёт команду списка пользователей.
func (c *Commands) newListUsersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-users",
		Short: "Список пользователей",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			users, err := c.adminClient.ListUsers(cmd.Context())
			if err != nil {
				return adminError(err, "")
			}
			if len(users) == 0 {
				fmt.Println("Пользователей нет.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ЛОГИН\tСОЗДАН\tСЕКРЕТОВ\tПАРОЛЬ")
			for _, u := range users {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", u.Login, u.CreatedAt.Local().Format("2006-01-02 15:04"),
					u.SecretCount, passwordStatus(u))
			}
			return w.Flush()
		},
	}
}

// newUserInfoCmd создаёт команду сведений о пользователе.
func (c *Commands) newUserInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "user-info <login>",
		Short: "Сведения о пользователе",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := c.adminClient.GetUser(cmd.Context(), args[0])
			if err != nil {
				return adminError(err, args[0])
			}
			fmt.Printf("Логин:      %s\n", u.Login)
			fmt.Printf("Создан:     %s\n", u.CreatedAt.Local().Format("2006-01-02 15:04"))
			fmt.Printf("Записей:    %d\n", u.SecretCount)
			fmt.Printf("Пароль:     %s\n", passwordStatus(u))
			fmt.Printf("Argon2id:   t=%d, %d MiB\n", u.KDF.Time, u.KDF.MemoryKiB/1024)
			return nil
		},
	}
}

// newRevokeSessionsCmd создаёт команду завершения всех сессий пользователя.
func (c *Commands) newRevokeSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke-sessions <login>",
		Short: "Завершить все сеансы пользователя (например, при потере устройства)",
		Long: "Отзывает все токены пользователя: на всех устройствах потребуется ввести\n" +
			"мастер-пароль заново. Пароль и записи не меняются.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.adminClient.RevokeSessions(cmd.Context(), args[0]); err != nil {
				return adminError(err, args[0])
			}
			fmt.Printf("Все сеансы пользователя %s завершены.\n", args[0])
			return nil
		},
	}
}

// passwordStatus описывает пароль пользователя для администратора.
func passwordStatus(u *domain.UserInfo) string {
	if u.PasswordExpiresAt == nil {
		return "постоянный"
	}
	if time.Now().After(*u.PasswordExpiresAt) {
		return "временный, истёк " + u.PasswordExpiresAt.Local().Format("2006-01-02 15:04")
	}
	return "временный до " + u.PasswordExpiresAt.Local().Format("2006-01-02 15:04")
}

// adminError переводит ошибки административных команд в понятные сообщения.
func adminError(err error, login string) error {
	switch {
	case errors.Is(err, domain.ErrNotSupported):
		return errNotAdminPort
	case errors.Is(err, domain.ErrNotFound) && login != "":
		return fmt.Errorf("пользователь %s не найден", login)
	default:
		return err
	}
}

// errNotAdminPort — административная команда отправлена на публичный порт.
var errNotAdminPort = errors.New("сервер не принимает административные команды на этом порту: " +
	"подключитесь к ADMIN_PORT (например, SHRON_SERVER=localhost:50052)")
