package command

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
)

// OutputOptions описывает параметры вывода секрета.
type OutputOptions struct {
	JSON       bool   // вывод в JSON вместо человекочитаемого
	OutputPath string // путь к файлу; если пусто — вывод в stdout
}

// WriteSecret выводит один секрет согласно опциям.
// Для binary с указанным OutputPath записывает сырые байты данных,
// для остальных типов — отформатированный текст (или JSON).
func WriteSecret(info *domain.SecretInfo, opts OutputOptions) error {
	// Binary + --output: записываем сырые данные в файл.
	if info.Type == domain.SecretTypeBinary && opts.OutputPath != "" {
		var b domain.BinarySecret
		if err := json.Unmarshal(info.Data, &b); err != nil {
			return fmt.Errorf("decode binary: %w", err)
		}
		return os.WriteFile(opts.OutputPath, b.Data, 0600)
	}

	var (
		output []byte
		err    error
	)
	if opts.JSON {
		output, err = json.MarshalIndent(info, "", "  ")
	} else {
		output, err = formatPretty(info)
	}
	if err != nil {
		return err
	}

	if opts.OutputPath != "" {
		return os.WriteFile(opts.OutputPath, output, 0600)
	}
	_, err = os.Stdout.Write(append(output, '\n'))
	return err
}

// ListFilter — условия отбора секретов в списке; пустое поле не ограничивает.
type ListFilter struct {
	Type  string
	Group string // без учёта регистра
}

// WriteSecretList выводит таблицу с метаинформацией секретов.
// Значения не выводятся — только группа, имя, тип, метаданные и дата обновления.
// Секреты упорядочены по группам, секреты без группы — в конце.
func WriteSecretList(secrets []*domain.SecretInfo, filter ListFilter) error {
	var t domain.SecretType
	if filter.Type != "" {
		var err error
		if t, err = domain.ParseSecretType(filter.Type); err != nil {
			return err
		}
	}
	group, err := domain.NormalizeGroup(filter.Group)
	if err != nil {
		return err
	}
	filtered := make([]*domain.SecretInfo, 0, len(secrets))
	for _, s := range secrets {
		if (t == "" || s.Type == t) && (group == "" || strings.EqualFold(s.Group, group)) {
			filtered = append(filtered, s)
		}
	}
	slices.SortStableFunc(filtered, func(a, b *domain.SecretInfo) int {
		if (a.Group == "") != (b.Group == "") {
			if a.Group == "" {
				return 1
			}
			return -1
		}
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Group), strings.ToLower(b.Group)),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ГРУППА\tИМЯ\tТИП\tМЕТАДАННЫЕ\tОБНОВЛЁН"); err != nil {
		return err
	}
	for _, s := range filtered {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.Group, s.Name, s.Type, s.Metadata,
			s.UpdatedAt.Format("2006-01-02 15:04"),
		); err != nil {
			return err
		}
	}
	return w.Flush()
}

// formatPretty форматирует секрет в человекочитаемом виде в зависимости от типа.
func formatPretty(info *domain.SecretInfo) ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Имя:        %s\n", info.Name)
	fmt.Fprintf(&buf, "Тип:        %s\n", info.Type)
	if info.Group != "" {
		fmt.Fprintf(&buf, "Группа:     %s\n", info.Group)
	}
	if info.Metadata != "" {
		fmt.Fprintf(&buf, "Метаданные: %s\n", info.Metadata)
	}
	fmt.Fprintf(&buf, "Создан:     %s\n", info.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&buf, "Обновлён:   %s\n", info.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintln(&buf, "---")

	switch info.Type {
	case domain.SecretTypeCredentials:
		var c domain.CredentialsSecret
		if err := json.Unmarshal(info.Data, &c); err != nil {
			return nil, fmt.Errorf("decode credentials: %w", err)
		}
		fmt.Fprintf(&buf, "Логин:    %s\n", c.Login)
		fmt.Fprintf(&buf, "Пароль:   %s\n", c.Password)
		if len(c.URIs) > 0 {
			fmt.Fprintf(&buf, "Сайты:    %s\n", strings.Join(c.URIs, ", "))
		}

	case domain.SecretTypeText:
		var t domain.TextSecret
		if err := json.Unmarshal(info.Data, &t); err != nil {
			return nil, fmt.Errorf("decode text: %w", err)
		}
		fmt.Fprintln(&buf, t.Text)

	case domain.SecretTypeCard:
		var card domain.CardSecret
		if err := json.Unmarshal(info.Data, &card); err != nil {
			return nil, fmt.Errorf("decode card: %w", err)
		}
		fmt.Fprintf(&buf, "Номер:     %s\n", card.Number)
		fmt.Fprintf(&buf, "Держатель: %s\n", card.Holder)
		fmt.Fprintf(&buf, "Срок:      %s\n", card.Expiry)
		fmt.Fprintf(&buf, "CVV:       %s\n", card.CVV)

	case domain.SecretTypeBinary:
		var b domain.BinarySecret
		if err := json.Unmarshal(info.Data, &b); err != nil {
			return nil, fmt.Errorf("decode binary: %w", err)
		}
		fmt.Fprintf(&buf, "Размер: %d байт\n", len(b.Data))
		fmt.Fprintln(&buf, "Используйте --output=<path> для сохранения в файл")

	default:
		return nil, fmt.Errorf("неизвестный тип секрета: %s", info.Type)
	}

	return buf.Bytes(), nil
}
