package command

import (
	"fmt"
	"strings"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/spf13/cobra"
)

// newSecretCmd создаёт команду-контейнер для работы с секретами.
// Объединяет подкоманды: create, get, update, delete, list.
func (c *Commands) newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Управление записями",
	}
	cmd.AddCommand(
		c.newCreateCmd(),
		c.newUpdateCmd(),
		c.newGetCmd(),
		c.newListCmd(),
		c.newDeleteCmd(),
	)
	return cmd
}

// newCreateCmd создаёт подкоманду создания нового секрета.
// Тип секрета определяет, какие поля будут запрошены интерактивно.
func (c *Commands) newCreateCmd() *cobra.Command {
	var name, secretType, group string
	var generate bool
	var gen genFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Создать запись",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := domain.ParseSecretType(secretType)
			if err != nil {
				return err
			}
			group, err := domain.NormalizeGroup(group)
			if err != nil {
				return err
			}

			generated, err := generatedPassword(generate, t, gen)
			if err != nil {
				return err
			}

			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}

			data, err := promptSecretData(t, generated)
			if err != nil {
				return err
			}

			metadata, err := promptSecretMetadata()
			if err != nil {
				return err
			}

			if err := secretSvc.CreateSecret(cmd.Context(), &domain.SecretPayload{
				Name:     name,
				Type:     t,
				Data:     data,
				Metadata: metadata,
				Group:    group,
			}); err != nil {
				return err
			}
			fmt.Println("Запись создана")
			if generated != "" {
				fmt.Printf("Сгенерированный пароль: %s\n", generated)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Название записи (обязательно)")
	cmd.Flags().StringVar(&secretType, "type", "", "Тип: credentials|text|card|binary (обязательно)")
	cmd.Flags().StringVar(&group, "group", "", "Папка записи (по желанию)")
	cmd.Flags().BoolVar(&generate, "generate", false, "сгенерировать пароль вместо ввода (для credentials)")
	gen.register(cmd)
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// newUpdateCmd создаёт подкоманду изменения секрета.
// Тип секрета определяет, какие поля будут запрошены интерактивно.
func (c *Commands) newUpdateCmd() *cobra.Command {
	var name, secretType, group string
	var generate bool
	var gen genFlags
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Изменить запись",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := domain.ParseSecretType(secretType)
			if err != nil {
				return err
			}
			group, err := domain.NormalizeGroup(group)
			if err != nil {
				return err
			}

			generated, err := generatedPassword(generate, t, gen)
			if err != nil {
				return err
			}

			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}

			// Без --group секрет остаётся в своей группе: остальные поля вводятся
			// заново, а группу проще не терять при каждом изменении пароля.
			if !cmd.Flags().Changed("group") {
				current, err := secretSvc.GetSecret(cmd.Context(), name, t)
				if err != nil {
					return err
				}
				group = current.Group
			}

			data, err := promptSecretData(t, generated)
			if err != nil {
				return err
			}

			metadata, err := promptSecretMetadata()
			if err != nil {
				return err
			}

			if err := secretSvc.UpdateSecret(cmd.Context(), &domain.SecretPayload{
				Name:     name,
				Type:     t,
				Data:     data,
				Metadata: metadata,
				Group:    group,
			}); err != nil {
				return err
			}
			fmt.Println("Запись изменена")
			if generated != "" {
				fmt.Printf("Сгенерированный пароль: %s\n", generated)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Название записи (обязательно)")
	cmd.Flags().StringVar(&secretType, "type", "", "Тип: credentials|text|card|binary (обязательно)")
	cmd.Flags().StringVar(&group, "group", "", `Папка записи: без флага остаётся прежняя, --group "" убирает из папки`)
	cmd.Flags().BoolVar(&generate, "generate", false, "сгенерировать пароль вместо ввода (для credentials)")
	gen.register(cmd)
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// newGetCmd создаёт подкоманду получения секрета.
// Поддерживает форматирование (--json) и сохранение в файл (--output).
// Для binary с --output записывает сырые байты, для остальных типов — отформатированный текст.
func (c *Commands) newGetCmd() *cobra.Command {
	var name, secretType string
	var jsonOutput bool
	var outputPath string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Показать запись",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := domain.ParseSecretType(secretType)
			if err != nil {
				return err
			}

			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}

			info, err := secretSvc.GetSecret(cmd.Context(), name, t)
			if err != nil {
				return err
			}

			return WriteSecret(info, OutputOptions{JSON: jsonOutput, OutputPath: outputPath})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Название записи (обязательно)")
	cmd.Flags().StringVar(&secretType, "type", "", "Тип: credentials|text|card|binary (обязательно)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Вывести в формате JSON")
	cmd.Flags().StringVar(&outputPath, "output", "", "Сохранить в файл (для binary — сырые данные)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

// newListCmd создаёт подкоманду вывода списка секретов пользователя.
// Выводит таблицу с метаинформацией (группа, имя, тип, метаданные, дата обновления), без
// значений секретов. Поддерживает фильтрацию по типу (--type) и группе (--group).
func (c *Commands) newListCmd() *cobra.Command {
	var filterType, filterGroup string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Список записей",
		RunE: func(cmd *cobra.Command, args []string) error {
			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}

			secrets, err := secretSvc.ListSecrets(cmd.Context())
			if err != nil {
				return err
			}

			return WriteSecretList(secrets, ListFilter{Type: filterType, Group: filterGroup})
		},
	}
	cmd.Flags().StringVar(&filterType, "type", "", "Фильтр по типам: credentials|text|card|binary")
	cmd.Flags().StringVar(&filterGroup, "group", "", "Только записи этой папки")
	return cmd
}

// newDeleteCmd создаёт подкоманду удаления секрета.
// По умолчанию запрашивает интерактивное подтверждение; флаг --yes пропускает его.
func (c *Commands) newDeleteCmd() *cobra.Command {
	var name, secretType string
	var skipConfirm bool
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Удалить запись",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := domain.ParseSecretType(secretType)
			if err != nil {
				return err
			}

			secretSvc, err := c.unlockSecretService(cmd.Context())
			if err != nil {
				return err
			}

			if !skipConfirm {
				confirm, err := promptLine(fmt.Sprintf("Удалить запись %q типа %s? [y/N]: ", name, t))
				if err != nil {
					return err
				}
				if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
					fmt.Println("Отменено")
					return nil
				}
			}

			if err := secretSvc.DeleteSecret(cmd.Context(), name, t); err != nil {
				return err
			}
			fmt.Printf("Запись %q (%s) удалена\n", name, t)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Название записи (обязательно)")
	cmd.Flags().StringVar(&secretType, "type", "", "Тип: credentials|text|card|binary (обязательно)")
	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Пропустить подтверждение")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
